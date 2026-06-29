#!/usr/bin/env python3
"""AI-powered evaluation and regression verification for mdflow.

Feeds CommonMark spec examples through mdflow (and glow as reference),
captures terminal output, and asks an LLM to judge rendering quality.

Usage:
    ai_test.py run    [filters]   AI eval (mdflow + glow), save results
    ai_test.py show   [filters]   display results (section / detail / single)
    ai_test.py diff   [filters]   regression check (no AI) — diffs current output vs saved snapshot

Filters (all commands):
    <section>        e.g. Tabs, "ATX headings"
    <number>         e.g. 23, 1,3,5, 1-10
    ALL              all entries
    --pass           only pass=true
    --fail           only pass=false
    --json           machine-readable output (show / diff only)

Data files:
    test/spec.json       CommonMark spec examples (read-only)
    test/weights.json    example weights (run ai_weights.py first)
    test/ai_results.json evaluation results (created/updated by run)

Environment:
    OPENAI_API_KEY       required for run command
"""

import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

from openai import OpenAI

from ai_prompts import build_eval_prompt, get_section_context

# ── Paths ──────────────────────────────────────────────────────────

SCRIPT_DIR = Path(__file__).resolve().parent
SPEC_PATH = SCRIPT_DIR / "spec.json"
RESULTS_PATH = SCRIPT_DIR / "ai_results.json"
WEIGHTS_PATH = SCRIPT_DIR / "weights.json"
MDFLOW_BIN = SCRIPT_DIR.parent / "mdflow"
GLOW_STYLE = SCRIPT_DIR / "glow_clean.json"

# ── ANSI helpers ───────────────────────────────────────────────────

BOLD = "\033[1m"
DIM = "\033[2m"
RED = "\033[31m"
GREEN = "\033[32m"
YELLOW = "\033[33m"
CYAN = "\033[36m"
RESET = "\033[0m"

_use_color = sys.stdout.isatty()


def _c(code: str, text: str) -> str:
    return f"{code}{text}{RESET}" if _use_color else text


def _pass_fail(val: bool) -> str:
    if val is True:
        return _c(GREEN, "Pass")
    if val is False:
        return _c(RED, "Fail")
    return "?"


def _bar(width: int, char: str = "\u2500") -> str:
    return char * width


# ── Data loading ───────────────────────────────────────────────────
#
# Example numbering: spec.json, weights.json, and ai_results.json all identify
# examples by the (section, example) pair. Both ai_weights.py and ai_test.py
# read the same spec.json, so example numbers and section names are always in
# sync. spec.json was extracted from dev_docs/commonMark_spec.txt. The weights
# script and the test script both key off the same (section, example) tuple
# from spec.json — no numbering drift is possible.


def _normalized_weighted_avg(entries: list) -> dict:
    """Compute per-section band-normalized weighted averages.

    Weights are bucketed into 10 bands (0.1 … 1.0).  Within each section
    every band that has data gets one equal vote — so a single w=1.0
    example does not dominate 100 w=0.1 examples, and sections with skewed
    weight distributions are not unfairly over- or under-represented.

    Global average averages the per-section averages (each section = 1 vote).
    """
    BANDS = [round(b * 0.1, 1) for b in range(1, 11)]  # [0.1, 0.2, …, 1.0]

    # Group by (section, band)
    bucket = {}  # key: (section, band) -> ([mdflow_scores], [glow_scores])
    for e in entries:
        sec = e["section"]
        raw_w = e.get("weight", 0.5)
        band = round(raw_w, 1)
        if band < 0.1:
            band = 0.1
        elif band > 1.0:
            band = 1.0
        key = (sec, band)
        if key not in bucket:
            bucket[key] = ([], [])
        bucket[key][0].append(e.get("mdflow_score", 0))
        bucket[key][1].append(e.get("glow_score", 0))

    # Per-section: average of band averages
    section_band_avgs = {}  # sec -> (sum_of_band_avgs_md, sum_of_band_avgs_gl, band_count)
    for (sec, band), (m_list, g_list) in bucket.items():
        m_band = sum(m_list) / len(m_list) if m_list else 0
        g_band = sum(g_list) / len(g_list) if g_list else 0
        if sec not in section_band_avgs:
            section_band_avgs[sec] = (0.0, 0.0, 0)
        s_m, s_g, cnt = section_band_avgs[sec]
        section_band_avgs[sec] = (s_m + m_band, s_g + g_band, cnt + 1)

    section_avgs = {}
    global_md = global_gl = 0.0
    for sec, (sum_m, sum_g, cnt) in section_band_avgs.items():
        section_avgs[sec] = (sum_m / cnt, sum_g / cnt) if cnt else (0.0, 0.0)

    if section_avgs:
        global_md = sum(v[0] for v in section_avgs.values()) / len(section_avgs)
        global_gl = sum(v[1] for v in section_avgs.values()) / len(section_avgs)

    return {"sections": section_avgs, "global": (global_md, global_gl)}


def load_spec() -> list:
    with open(SPEC_PATH, encoding="utf-8") as f:
        return json.load(f)


def load_results() -> list:
    if not RESULTS_PATH.exists():
        return []
    with open(RESULTS_PATH, encoding="utf-8") as f:
        return json.load(f)


def save_results(results: list):
    with open(RESULTS_PATH, "w", encoding="utf-8") as f:
        json.dump(results, f, indent=2, ensure_ascii=False)


def load_weights() -> dict:
    """Load weights, keyed by (section, example)."""
    if not WEIGHTS_PATH.exists():
        return {}
    with open(WEIGHTS_PATH, encoding="utf-8") as f:
        data = json.load(f)
    return {(e["section"], e["example"]): e for e in data}


def spec_section_order() -> list:
    seen = []
    for e in load_spec():
        if e["section"] not in seen:
            seen.append(e["section"])
    return seen


# ── Runners ────────────────────────────────────────────────────────


def run_mdflow(markdown: str) -> str:
    proc = subprocess.run(
        [str(MDFLOW_BIN)],
        input=markdown,
        capture_output=True,
        text=True,
        timeout=15,
    )
    if proc.returncode != 0:
        print(
            f"  [warn] mdflow exit {proc.returncode}: {proc.stderr[:200]}",
            file=sys.stderr,
        )
    return proc.stdout.rstrip("\n")


def run_glow(markdown: str) -> str:
    proc = subprocess.run(
        ["glow", "-s", str(GLOW_STYLE)],
        input=markdown,
        capture_output=True,
        text=True,
        timeout=15,
    )
    if proc.returncode != 0:
        print(
            f"  [warn] glow exit {proc.returncode}: {proc.stderr[:200]}",
            file=sys.stderr,
        )
    return proc.stdout.rstrip("\n")


# ── Filter system ──────────────────────────────────────────────────


def parse_filters(argv: list) -> dict:
    pos_args = []
    flags = []
    for a in argv:
        if a.startswith("--"):
            flags.append(a)
        else:
            pos_args.append(a)

    filters = {
        "sections": [],
        "examples": [],
        "verdicts": None,
        "json_out": False,
    }

    if "--json" in flags:
        filters["json_out"] = True

    verdicts = set()
    for f in flags:
        if f == "--pass":
            verdicts.add(True)
        elif f == "--fail":
            verdicts.add(False)
    if verdicts:
        filters["verdicts"] = verdicts

    known_sections = set(spec_section_order()) | {"ALL", "all"}
    for a in pos_args:
        if a.upper() == "ALL":
            continue
        if a in known_sections:
            filters["sections"].append(a)
        elif re.match(r"^\d+$", a):
            filters["examples"].append(int(a))
        elif re.match(r"^\d+-\d+$", a):
            lo, hi = a.split("-")
            filters["examples"].extend(range(int(lo), int(hi) + 1))
        elif re.match(r"^[\d,]+$", a):
            for part in a.split(","):
                part = part.strip()
                if part.isdigit():
                    filters["examples"].append(int(part))

    return filters


def apply_spec_filters(entries: list, filters: dict) -> list:
    result = entries
    if filters["sections"]:
        result = [e for e in result if e["section"] in filters["sections"]]
    if filters["examples"]:
        nums = set(filters["examples"])
        result = [e for e in result if e["example"] in nums]
    return result


def apply_result_filters(entries: list, filters: dict) -> list:
    result = entries
    if filters["sections"]:
        result = [e for e in result if e.get("section") in filters["sections"]]
    if filters["examples"]:
        nums = set(filters["examples"])
        result = [e for e in result if e.get("example") in nums]
    if filters["verdicts"] is not None:
        result = [e for e in result if e.get("pass") in filters["verdicts"]]
    return result


# ── Evaluation ─────────────────────────────────────────────────────


def evaluate(client: OpenAI, section: str, markdown: str, html: str,
             weight: float, weight_explanation: str,
             mdflow_out: str, glow_out: str) -> dict:
    ctx = get_section_context(section, max_chars=10000)
    prompt = build_eval_prompt(ctx, markdown, html, weight, weight_explanation,
                                mdflow_out, glow_out)
    response = client.chat.completions.create(
        model="gpt-5-mini",
        messages=[{"role": "user", "content": prompt}],
        response_format={"type": "json_object"},
    )
    return json.loads(response.choices[0].message.content)


# ═══════════════════════════════════════════════════════════════════
#  Commands
# ═══════════════════════════════════════════════════════════════════


def cmd_run(filters: dict):
    if not SPEC_PATH.exists():
        die(f"spec.json not found: {SPEC_PATH}")
    if not MDFLOW_BIN.exists():
        die(f"mdflow binary not found: {MDFLOW_BIN}")
    if not WEIGHTS_PATH.exists():
        die(f"weights.json not found. Run: python3 ai_weights.py ALL")

    api_key = os.environ.get("OPENAI_API_KEY", "")
    if not api_key:
        die("OPENAI_API_KEY environment variable is not set.")

    spec = load_spec()
    entries = apply_spec_filters(spec, filters)
    if not entries:
        die("No spec entries matched filters.")

    weights = load_weights()
    if not weights:
        die("weights.json is empty. Run: python3 ai_weights.py ALL")

    missing = sum(1 for e in entries if (e["section"], e["example"]) not in weights)
    if missing:
        print(f"Warning: {missing} entries have no weight — they will be skipped.",
              file=sys.stderr)

    sections = sorted(set(e["section"] for e in entries))
    label = ", ".join(sections) if len(sections) <= 3 else f"{len(sections)} sections"
    print(f"Run   {len(entries)} examples  ({label})")
    print(f"Model gpt-5-mini")
    print()

    client = OpenAI(api_key=api_key)
    results = load_results()
    result_map = {(e["section"], e["example"]): e for e in results}

    changed = 0
    skipped = 0

    for i, entry in enumerate(entries):
        example_num = entry["example"]
        section = entry["section"]
        markdown = entry["markdown"]
        html = entry["html"]
        key = (section, example_num)

        # Get weight
        w = weights.get(key, {})
        weight_val = w.get("weight", 0.5)
        weight_expl = w.get("explanation", "no weight data")

        # Run both renderers
        mdflow_terminal = run_mdflow(markdown)
        glow_terminal = run_glow(markdown)

        # Skip: output unchanged (AI result would be the same)
        existing = result_map.get(key)
        if existing and mdflow_terminal == existing.get("mdflow_terminal", ""):
            skipped += 1
            print(f"[{i + 1:>3}/{len(entries)}] Skip   #{example_num} {section:<30} "
                  f"(unchanged)")
            continue

        action = "Update" if key in result_map else "New"

        print(f"[{i + 1:>3}/{len(entries)}] {action:<6} #{example_num} {section:<30} ",
              end="", flush=True)

        try:
            llm = evaluate(client, section, markdown, html,
                           weight_val, weight_expl,
                           mdflow_terminal, glow_terminal)
        except Exception as exc:
            print(f"FAIL: {exc}")
            llm = {
                "a_content": 0, "a_structure": 0, "a_style": 0, "a_score": 0,
                "b_content": 0, "b_structure": 0, "b_style": 0, "b_score": 0,
                "pass": False,
                "issues": [str(exc)],
                "explanation": f"LLM evaluation error: {exc}",
            }

        result = {
            "section": section,
            "example": example_num,
            "markdown": markdown,
            "html": html,
            "weight": weight_val,
            "weight_explanation": weight_expl,
            "mdflow_terminal": mdflow_terminal,
            "mdflow_content": llm.get("a_content", 0),
            "mdflow_structure": llm.get("a_structure", 0),
            "mdflow_style": llm.get("a_style", 0),
            "mdflow_score": llm.get("a_score", 0),
            "glow_terminal": glow_terminal,
            "glow_content": llm.get("b_content", 0),
            "glow_structure": llm.get("b_structure", 0),
            "glow_style": llm.get("b_style", 0),
            "glow_score": llm.get("b_score", 0),
            "pass": llm.get("pass", False),
            "issues": llm.get("issues", []),
            "explanation": llm.get("explanation", ""),
        }

        result_map[key] = result
        changed += 1

        print(f"mdflow={result['mdflow_score']}  glow={result['glow_score']}  "
              f"weight={weight_val:.1f}  {_pass_fail(result['pass'])}")

        # Save incrementally
        save_results(list(result_map.values()))
        time.sleep(0.15)

    # Final save
    save_results(list(result_map.values()))

    # Summary of what we just ran
    ran_keys = {(e["section"], e["example"]) for e in entries}
    ran = [result_map[k] for k in ran_keys if k in result_map]

    print()
    print(f"Saved  {RESULTS_PATH}  ({len(result_map)} total, "
          f"{changed} updated, {skipped} skipped)")
    _print_run_summary(ran)


def cmd_show(filters: dict):
    results = load_results()

    if not results:
        print("No results yet. Run: ai_test.py run <section>")
        return

    filtered = apply_result_filters(results, filters)
    if not filtered:
        print("No results matched filters.")
        return

    if filters["json_out"]:
        print(json.dumps(filtered, indent=2, ensure_ascii=False))
        return

    has_section_filter = bool(filters["sections"])
    has_example_filter = bool(filters["examples"])
    has_verdict_filter = filters["verdicts"] is not None
    has_any_filter = has_section_filter or has_example_filter or has_verdict_filter

    if has_example_filter:
        for entry in filtered:
            _print_example_detail(entry)
    elif has_section_filter and len(filters["sections"]) == 1 and not has_verdict_filter:
        _print_section_detail(results, filters["sections"][0])
    elif has_verdict_filter and not has_section_filter and not has_example_filter:
        _print_section_table(results, filtered)
        print()
        label = "pass" if True in filters["verdicts"] else "fail"
        _print_example_list(filtered, title=f"All {label}")
    elif has_section_filter and has_verdict_filter:
        label = f"{filters['sections'][0]} (fail)" if False in filters["verdicts"] else f"{filters['sections'][0]} (pass)"
        _print_example_list(filtered, title=label)
    elif has_any_filter:
        _print_example_list(filtered, title=f"Matched {len(filtered)} examples")
    else:
        _print_section_table(results, filtered)


def cmd_diff(filters: dict):
    results = load_results()
    if not results:
        print("No results yet. Run: ai_test.py run <section>")
        return

    filtered = apply_result_filters(results, filters)
    if not filtered:
        print("No results matched filters.")
        return

    print(f"Diff  {len(filtered)} entries")
    print()

    same_count = 0
    diff_count = 0
    skip_count = 0
    diffs = []

    for i, entry in enumerate(filtered):
        example_num = entry.get("example", "?")
        section = entry.get("section", "?")
        markdown = entry.get("markdown", "")
        previous = entry.get("mdflow_terminal", "")
        prev_pass = entry.get("pass", "?")
        prev_m = entry.get("mdflow_score", "?")

        if not previous:
            skip_count += 1
            print(f"[{i + 1:>3}/{len(filtered)}] SKIP  #{example_num} {section}")
            continue

        current = run_mdflow(markdown)

        if current == previous:
            same_count += 1
            print(f"[{i + 1:>3}/{len(filtered)}] SAME  #{example_num} {section}")
            continue

        diff_count += 1
        print(f"[{i + 1:>3}/{len(filtered)}] DIFF  #{example_num} {section}  "
              f"prev={prev_m}  {_pass_fail(prev_pass)}")
        diffs.append({
            "example": example_num,
            "section": section,
            "previous": previous,
            "current": current,
            "prev_mdflow_score": prev_m,
            "prev_pass": prev_pass,
        })

    if diffs:
        for d in diffs:
            print()
            print(f"{_bar(70)}")
            print(f"  Example {d['example']}  section={d['section']}  "
                  f"prev_mdflow={d['prev_mdflow_score']}  {_pass_fail(d['prev_pass'])}")
            print(f"{_bar(70)}")
            print()
            print("--- PREVIOUS (ai_results.json) ---")
            print(repr(d["previous"]))
            print()
            print("--- CURRENT (mdflow) ---")
            print(repr(d["current"]))

    print()
    print(f"Summary  {len(filtered)} entries  "
          f"{same_count} same  {diff_count} diff  {skip_count} skipped")


# ── Display helpers ────────────────────────────────────────────────


def die(msg: str):
    print(f"Error: {msg}", file=sys.stderr)
    sys.exit(1)


def _print_run_summary(entries: list):
    passes = sum(1 for e in entries if e.get("pass") is True)
    fails = sum(1 for e in entries if e.get("pass") is False)
    m_scores = [e.get("mdflow_score", 0) for e in entries]
    g_scores = [e.get("glow_score", 0) for e in entries]
    weights = [e.get("weight", 0.5) for e in entries]
    total_w = sum(weights) if weights else 0
    wm_avg = sum(m_scores[i] * weights[i] for i in range(len(weights))) / total_w if total_w else 0
    wg_avg = sum(g_scores[i] * weights[i] for i in range(len(weights))) / total_w if total_w else 0
    print(f"Summary  {_c(GREEN, str(passes) + ' pass')}  "
          f"{_c(RED, str(fails) + ' fail')}  "
          f"mdflow={wm_avg:.1f}/3  glow={wg_avg:.1f}/3")


def _print_section_table(all_results: list, filtered_results: list):
    spec = load_spec()
    section_order = spec_section_order()

    spec_counts = {}
    for e in spec:
        spec_counts[e["section"]] = spec_counts.get(e["section"], 0) + 1

    result_map = {(e["section"], e["example"]): e for e in all_results}

    # Normalized weighted averages (per-section weight normalization)
    norm = _normalized_weighted_avg(all_results)
    section_avgs = norm["sections"]
    global_md, global_gl = norm["global"]

    section_stats = []
    for sec in section_order:
        total = spec_counts.get(sec, 0)
        sec_results = [e for e in all_results if e["section"] == sec]
        done = len(sec_results)
        p_count = sum(1 for e in sec_results if e.get("pass") is True)
        f_count = sum(1 for e in sec_results if e.get("pass") is False)
        md_avg = section_avgs.get(sec, (0, 0))[0]
        gl_avg = section_avgs.get(sec, (0, 0))[1]
        section_stats.append((sec, total, done, p_count, f_count, md_avg, gl_avg))

    print()
    print(_c(BOLD, "mdflow AI Test Results"))
    total_all = sum(s[1] for s in section_stats)
    total_done = sum(s[2] for s in section_stats)
    all_p = sum(s[3] for s in section_stats)
    all_f = sum(s[4] for s in section_stats)

    if total_done > 0:
        tested_sections = sum(1 for s in section_stats if s[2] > 0)
        print(f"Total: {total_done}/{total_all}  "
              f"{_c(GREEN, str(all_p) + ' pass')}  "
              f"{_c(RED, str(all_f) + ' fail')}  "
              f"mdflow={global_md:.1f}/3  glow={global_gl:.1f}/3  "
              f"({tested_sections} sections)")
    else:
        print(f"Total: 0/{total_all} tested")
    print()

    header = f" {'#':<2}  {'Section':<36} {'Total':>5} {'Done':>5} {'Pass':>5} {'Fail':>5}  {'mdflow':>6} {'glow':>6}"
    print(_c(BOLD, header))
    print(_c(DIM, _bar(len(header))))

    for i, (sec, total, done, p_count, f_count, md_avg, gl_avg) in enumerate(section_stats):
        idx = f"{i + 1:>2}"
        done_str = str(done) if done > 0 else "-"
        p_str = str(p_count) if done > 0 else "-"
        f_str = str(f_count) if done > 0 else "-"
        md_str = f"{md_avg:>5.1f}" if done > 0 else "-"
        gl_str = f"{gl_avg:>5.1f}" if done > 0 else "-"

        if done > 0 and f_count == 0:
            color = GREEN
        elif done > 0 and f_count > 0:
            color = YELLOW
        else:
            color = DIM

        row = f" {idx}  {sec:<36} {total:>5} {done_str:>5} {p_str:>5} {f_str:>5}  {md_str:>6} {gl_str:>6}"
        print(_c(color, row))

    print(_c(DIM, _bar(len(header))))
    row_total = f" {'':<2}  {'26 sections':<36} {total_all:>5} {total_done:>5} {all_p:>5} {all_f:>5}"
    print(_c(DIM, row_total))


def _print_section_detail(all_results: list, section: str):
    spec = load_spec()
    spec_entries = [e for e in spec if e["section"] == section]
    result_map = {(e["section"], e["example"]): e for e in all_results}

    tested = [result_map[(section, e["example"])] for e in spec_entries
              if (section, e["example"]) in result_map]

    # Band-normalized weighted avg for this section
    wm_avg = wg_avg = 0
    if tested:
        p_count = sum(1 for e in tested if e.get("pass") is True)
        f_count = sum(1 for e in tested if e.get("pass") is False)
        # Bucket into weight bands
        bucket = {}
        for e in tested:
            raw_w = e.get("weight", 0.5)
            band = round(raw_w, 1)
            if band < 0.1:
                band = 0.1
            elif band > 1.0:
                band = 1.0
            if band not in bucket:
                bucket[band] = ([], [])
            bucket[band][0].append(e.get("mdflow_score", 0))
            bucket[band][1].append(e.get("glow_score", 0))
        if bucket:
            wm_avg = sum(sum(m) / len(m) for m, _ in bucket.values()) / len(bucket)
            wg_avg = sum(sum(g) / len(g) for _, g in bucket.values()) / len(bucket)
    else:
        p_count = f_count = 0

    max_num = max((e["example"] for e in spec_entries), default=0)
    num_width = len(str(max_num))

    print()
    print(_c(BOLD, f"Section: {section}  "
          f"({len(tested)}/{len(spec_entries)} tested)"))
    print(f"  {_c(GREEN, str(p_count) + ' pass')}  "
          f"{_c(RED, str(f_count) + ' fail')}  "
          f"mdflow={wm_avg:.1f}/3  glow={wg_avg:.1f}/3")
    print()

    for e in spec_entries:
        num = e["example"]
        key = (section, num)
        if key in result_map:
            r = result_map[key]
            m_score = r.get("mdflow_score", "?")
            g_score = r.get("glow_score", "?")
            weight = r.get("weight", "?")
            passed = r.get("pass", "?")

            print(f"  {'#' + str(num):>{num_width + 1}}  "
                  f"weight={weight:.1f}  mdflow={m_score}  glow={g_score}  "
                  f"{_pass_fail(passed)}")
        else:
            print(f"  {'#' + str(num):>{num_width + 1}}  "
                  f"{_c(DIM, '- not tested -')}")


def _print_example_list(entries: list, title: str = ""):
    if title:
        print()
        print(_c(BOLD, title))
    if not entries:
        print("  (none)")
        return

    print()
    max_num = max((e.get("example", 0) for e in entries), default=0)
    num_width = len(str(max_num))

    for e in entries:
        num = e.get("example", "?")
        section = e.get("section", "?")
        weight = e.get("weight", "?")
        m_score = e.get("mdflow_score", "?")
        g_score = e.get("glow_score", "?")
        passed = e.get("pass", "?")

        print(f"  {'#' + str(num):>{num_width + 1}}  "
              f"weight={weight:.1f}  mdflow={m_score}  glow={g_score}  "
              f"{_pass_fail(passed):<10}  {_c(DIM, section)}")


def _print_example_detail(entry: dict):
    section = entry.get("section", "?")
    example = entry.get("example", "?")
    weight = entry.get("weight", "?")
    weight_expl = entry.get("weight_explanation", "")
    passed = entry.get("pass", "?")
    m_content = entry.get("mdflow_content", "?")
    m_structure = entry.get("mdflow_structure", "?")
    m_style = entry.get("mdflow_style", "?")
    m_score = entry.get("mdflow_score", "?")
    g_content = entry.get("glow_content", "?")
    g_structure = entry.get("glow_structure", "?")
    g_style = entry.get("glow_style", "?")
    g_score = entry.get("glow_score", "?")
    issues = entry.get("issues", [])
    explanation = entry.get("explanation", "")
    markdown = entry.get("markdown", "")
    html = entry.get("html", "")
    md_terminal = entry.get("mdflow_terminal", "")
    gl_terminal = entry.get("glow_terminal", "")

    W = 70

    print()
    print(_c(BOLD, f"Example #{example}  {section}  "
          f"weight={weight:.1f}  {_pass_fail(passed)}"))
    print(_bar(W))

    if weight_expl:
        print(f"  {_c(DIM, 'Weight:')} {weight_expl}")

    print()
    print(_c(BOLD, f"  {'':<12} content  structure  style  score"))
    print(f"  {'Mdflow':<12} {m_content:>7}  {m_structure:>9}  {m_style:>5}  {m_score:>5}/3")
    print(f"  {'Glow':<12} {g_content:>7}  {g_structure:>9}  {g_style:>5}  {g_score:>5}/3")

    if explanation:
        print()
        print(_c(BOLD, "  Explanation:"))
        for line in _wrap_text(explanation, W - 4):
            print(f"    {line}")

    if issues:
        print()
        print(_c(BOLD, "  Issues:"))
        for issue in issues:
            print(f"    - {issue}")

    if markdown:
        print()
        print(_c(DIM, f"  {'─' * (W - 4)}"))
        print(_c(BOLD, "  Markdown input:"))
        for line in markdown.rstrip("\n").split("\n"):
            print(f"    {line}" if line.strip() else f"    {_c(DIM, '(blank)')}")

    if html:
        print()
        print(_c(DIM, f"  {'─' * (W - 4)}"))
        print(_c(BOLD, "  Expected HTML:"))
        for line in html.rstrip("\n").split("\n")[:10]:
            if len(line) > W - 4:
                line = line[:W - 7] + "..."
            print(f"    {line}")

    if md_terminal:
        print()
        print(_c(DIM, f"  {'─' * (W - 4)}"))
        print(_c(BOLD, "  Mdflow terminal (repr):"))
        for line in repr(md_terminal).split("\\n")[:8]:
            if len(line) > W - 4:
                line = line[:W - 7] + "..."
            print(f"    {line}")

    if gl_terminal:
        print()
        print(_c(DIM, f"  {'─' * (W - 4)}"))
        print(_c(BOLD, "  Glow terminal:"))
        for line in gl_terminal.rstrip("\n").split("\n")[:8]:
            if len(line) > W - 4:
                line = line[:W - 7] + "..."
            print(f"    {line}")

    print()


def _wrap_text(text: str, width: int) -> list:
    words = text.split()
    lines = []
    current = ""
    for w in words:
        if len(current) + len(w) + 1 <= width:
            current = (current + " " + w) if current else w
        else:
            if current:
                lines.append(current)
            current = w
    if current:
        lines.append(current)
    return lines


# ═══════════════════════════════════════════════════════════════════
#  Main
# ═══════════════════════════════════════════════════════════════════


def main():
    if len(sys.argv) < 2:
        prog = os.path.basename(sys.argv[0])
        print(f"Usage: {prog} <command> [filters...]", file=sys.stderr)
        print(file=sys.stderr)
        print("Commands:", file=sys.stderr)
        print(f"  {prog} run     [filters]   AI eval (mdflow + glow), save results", file=sys.stderr)
        print(f"  {prog} show    [filters]   display results", file=sys.stderr)
        print(f"  {prog} diff    [filters]   regression check (no AI) — diffs current output vs saved snapshot", file=sys.stderr)
        print(file=sys.stderr)
        print("Filters:", file=sys.stderr)
        print("  <section>       e.g. Tabs, \"ATX headings\"", file=sys.stderr)
        print("  <number>        e.g. 23, 1,3,5, 1-10", file=sys.stderr)
        print("  ALL             all entries", file=sys.stderr)
        print("  --pass          only pass=true", file=sys.stderr)
        print("  --fail          only pass=false", file=sys.stderr)
        print("  --json          machine-readable output (show / diff)", file=sys.stderr)
        print(file=sys.stderr)
        print("Setup (one-time):", file=sys.stderr)
        print(f"  python3 ai_weights.py ALL    compute weights for all examples", file=sys.stderr)
        print(file=sys.stderr)
        print("Examples:", file=sys.stderr)
        print(f"  {prog} run Tabs --fail       re-run failed Tabs", file=sys.stderr)
        print(f"  {prog} run 23                run single example", file=sys.stderr)
        print(f"  {prog} show                  section summary", file=sys.stderr)
        print(f"  {prog} show Tabs             section detail", file=sys.stderr)
        print(f"  {prog} show Tabs 3           single example", file=sys.stderr)
        print(f"  {prog} show --fail           all failures", file=sys.stderr)
        print(f"  {prog} diff --fail           diff previously-failing examples", file=sys.stderr)
        sys.exit(1)

    command = sys.argv[1]
    raw_args = sys.argv[2:]

    if command not in ("run", "show", "diff"):
        print(f"Unknown command: {command}", file=sys.stderr)
        sys.exit(1)

    filters = parse_filters(raw_args)

    if command == "run":
        cmd_run(filters)
    elif command == "show":
        cmd_show(filters)
    elif command == "diff":
        cmd_diff(filters)


if __name__ == "__main__":
    main()
