#!/usr/bin/env python3
"""Single-pass AI evaluation for mdflow.

Usage:
    eval.py run    [filters]   AI eval (mdflow + glow), save results
    eval.py show   [filters]   Display results
    eval.py diff   [filters]   Regression check (no AI)

Filters:
    <section>          e.g. Tabs, "ATX headings"
    <number>           e.g. 23, 1,3,5, 1-10
    ALL                all entries
    --pass             only pass=true
    --fail             only pass=false
    --force            force re-run (skip unchanged check)
    --workers=N        parallel workers (default 3, max 10)

Output flags (show/diff):
    --json             machine-readable JSON output
    --raw              show raw code (repr) instead of rendered terminal output

Data files:
    eval_results.json   results (created/updated by run)
    prompts/general.json       shared prompt template
    prompts/sections/*.json    section-specific notes

Environment:
    OPENAI_API_KEY      required for run
"""

import concurrent.futures
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

from openai import OpenAI

# ── Paths ──────────────────────────────────────────────────────────

SCRIPT_DIR = Path(__file__).resolve().parent
SPEC_PATH = SCRIPT_DIR / "spec.json"
RESULTS_PATH = SCRIPT_DIR / "eval_results.json"
PROMPTS_DIR = SCRIPT_DIR / "prompts"
SECTIONS_DIR = PROMPTS_DIR / "sections"
MDFLOW_BIN = SCRIPT_DIR.parent / "mdflow"
GLOW_STYLE = SCRIPT_DIR / "glow_clean.json"

MODEL = "gpt-5.6-terra"

# ── Display helpers ───────────────────────────────────────────────

def _pass_fail(val: bool | None) -> str:
    if val is True:
        return "Pass"
    if val is False:
        return "Fail"
    return "?"


def _bar(width: int, char: str = "\u2500") -> str:
    return char * width


def _trim_name(name: str, max_len: int = 12) -> str:
    return name if len(name) <= max_len else name[:max_len - 1] + "\u2026"


def _reset_terminal():
    """Reset ANSI styles and ensure cursor is visible."""
    sys.stdout.write("\033[0m\033[?25h")
    sys.stdout.flush()


def _html_to_ansi(html: str) -> str | None:
    from html.parser import HTMLParser
    from io import StringIO
    from rich.console import Console
    from rich.style import Style
    from rich.text import Text

    class _HtmlRenderer(HTMLParser):
        def __init__(self):
            super().__init__()
            self._parts: list[Text | str] = []
            self._stack: list[dict] = []

        def _current_style(self) -> Style | None:
            kw = {}
            for s in self._stack:
                kw.update(s)
            return Style(**kw) if kw else None

        def handle_starttag(self, tag, attrs):
            s = {}
            if tag in ("strong", "b", "h1", "h2", "h3", "h4", "h5", "h6", "th"):
                s["bold"] = True
            if tag in ("em", "i", "cite", "var"):
                s["italic"] = True
            if tag in ("code", "tt", "pre"):
                s["color"] = "cyan"
            if tag in ("a",):
                s["underline"] = True
            if tag in ("del", "s"):
                s["strike"] = True
            if tag == "mark":
                s["bgcolor"] = "yellow"
            if tag == "pre":
                s["color"] = "dim white"
            if tag in ("h1",):
                s["color"] = "bright_yellow"
            if tag in ("h2",):
                s["color"] = "yellow"
            if tag in ("h3",):
                s["color"] = "cyan"
            self._stack.append(s)

        def handle_endtag(self, tag):
            if tag in ("p", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "tr"):
                self._parts.append("\n")
            if tag in ("td", "th"):
                self._parts.append("  ")
            if tag == "br":
                self._parts.append("\n")
            if tag == "hr":
                self._parts.append("─" * 40 + "\n")
            if self._stack:
                self._stack.pop()

        def handle_data(self, data):
            style = self._current_style()
            self._parts.append(Text(data, style=style) if style else data)

        def handle_entityref(self, name):
            import html
            self._parts.append(html.entities.html5.get(name, f"&{name};"))

        def handle_charref(self, name):
            import html
            try:
                code = int(name) if name.isdigit() else int(name[1:], 16)
                self._parts.append(chr(code))
            except (ValueError, OverflowError):
                self._parts.append(f"&#{name};")

        def get_ansi(self) -> str:
            buf = StringIO()
            console = Console(file=buf, force_terminal=True, width=999, highlight=False, color_system="truecolor")
            for part in self._parts:
                console.print(part, end="")
            return buf.getvalue()

    try:
        r = _HtmlRenderer()
        r.feed(html)
        r.close()
        return r.get_ansi()
    except Exception as exc:
        print(f"  [html_to_ansi error] {exc}", file=sys.stderr)
        return None


# ── Data loading ───────────────────────────────────────────────────


def read_json(path: Path, default=None):
    if not path.exists():
        return default
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def write_json(path: Path, data):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)


def _section_slug(section: str) -> str:
    return section.lower().replace(" ", "_").replace("&", "and").replace("'", "")


def build_prompt(section: str, markdown: str, html: str,
                 mdflow_out: str, glow_out: str) -> str:
    general = read_json(PROMPTS_DIR / "general.json", {})
    template = general.get("prompt", "")
    slug = _section_slug(section)
    section_data = read_json(SECTIONS_DIR / f"{slug}.json", {})
    notes = section_data.get("notes", "") if section_data else ""
    return (template
            .replace("__SECTION_NOTES__", notes)
            .replace("__MARKDOWN__", markdown)
            .replace("__HTML__", html)
            .replace("__MDFLOW_OUTPUT__", mdflow_out)
            .replace("__GLOW_OUTPUT__", glow_out))


def spec_section_order() -> list:
    spec = read_json(SPEC_PATH, [])
    seen = []
    for e in spec:
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
        "raw": False,
        "force": False,
        "workers": 3,
    }

    verdicts = set()
    for f in flags:
        if f == "--pass":
            verdicts.add(True)
        elif f == "--fail":
            verdicts.add(False)
        elif f == "--json":
            filters["json_out"] = True
        elif f == "--raw":
            filters["raw"] = True
        elif f == "--force":
            filters["force"] = True
        elif f.startswith("--workers="):
            try:
                filters["workers"] = min(int(f.split("=", 1)[1]), 10)
            except ValueError:
                pass
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


# ── Scoring helpers ────────────────────────────────────────────────


def _normalized_weighted_avg(entries: list) -> dict:
    BANDS = [round(b * 0.1, 1) for b in range(1, 11)]

    bucket = {}
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

    section_band_avgs = {}
    for (sec, band), (m_list, g_list) in bucket.items():
        m_band = sum(m_list) / len(m_list) if m_list else 0
        g_band = sum(g_list) / len(g_list) if g_list else 0
        if sec not in section_band_avgs:
            section_band_avgs[sec] = (0.0, 0.0, 0)
        s_m, s_g, cnt = section_band_avgs[sec]
        section_band_avgs[sec] = (s_m + m_band, s_g + g_band, cnt + 1)

    section_avgs = {}
    for sec, (sum_m, sum_g, cnt) in section_band_avgs.items():
        section_avgs[sec] = (sum_m / cnt, sum_g / cnt) if cnt else (0.0, 0.0)

    if section_avgs:
        global_md = sum(v[0] for v in section_avgs.values()) / len(section_avgs)
        global_gl = sum(v[1] for v in section_avgs.values()) / len(section_avgs)
    else:
        global_md = global_gl = 0.0

    return {"sections": section_avgs, "global": (global_md, global_gl)}


# ── Pass/fail computation ──────────────────────────────────────────


def _compute_pass(score: int, weight: float) -> bool:
    if score >= 2:
        return True
    if score == 1 and weight <= 0.3:
        return True
    return False


# ── Evaluation ─────────────────────────────────────────────────────


def evaluate(client: OpenAI, section: str, markdown: str, html: str,
             mdflow_out: str, glow_out: str) -> dict:
    prompt = build_prompt(section, markdown, html, mdflow_out, glow_out)
    response = client.chat.completions.create(
        model=MODEL,
        messages=[{"role": "user", "content": prompt}],
        response_format={"type": "json_object"},
    )
    return json.loads(response.choices[0].message.content)


def assemble_result(section: str, example_num: int,
                    markdown: str, html: str,
                    mdflow_terminal: str, glow_terminal: str,
                    llm: dict) -> dict:
    weight = llm.get("weight", 0.5)
    mdflow_score = llm.get("a_score", 0)
    return {
        "section": section,
        "example": example_num,
        "markdown": markdown,
        "html": html,
        "weight": weight,
        "weight_explanation": llm.get("weight_explanation", ""),
        "mdflow_terminal": mdflow_terminal,
        "mdflow_score": mdflow_score,
        "glow_terminal": glow_terminal,
        "glow_score": llm.get("b_score", 0),
        "pass": _compute_pass(mdflow_score, weight),
        "issues": llm.get("issues", []),
        "explanation": llm.get("explanation", ""),
        "evaluated_at": time.strftime("%Y-%m-%dT%H:%M:%S"),
    }


# ═══════════════════════════════════════════════════════════════════
#  Commands
# ═══════════════════════════════════════════════════════════════════


def _eval_one(api_key: str, entry: dict, result_map: dict,
              force: bool) -> tuple:
    """Evaluate a single example. Returns (key, result_or_None, action, is_error)."""
    example_num = entry["example"]
    section = entry["section"]
    markdown = entry["markdown"]
    html = entry["html"]
    key = (section, example_num)

    mdflow_terminal = run_mdflow(markdown)
    glow_terminal = run_glow(markdown)

    if not force:
        existing = result_map.get(key)
        if existing and mdflow_terminal == existing.get("mdflow_terminal", ""):
            return (key, None, "skip", False)

    action = "Update" if key in result_map else "New"

    try:
        client = OpenAI(api_key=api_key)
        prompt = build_prompt(section, markdown, html, mdflow_terminal, glow_terminal)
        response = client.chat.completions.create(
            model=MODEL,
            messages=[{"role": "user", "content": prompt}],
            response_format={"type": "json_object"},
        )
        llm = json.loads(response.choices[0].message.content)
    except Exception as exc:
        result = assemble_result(section, example_num, markdown, html,
                                 mdflow_terminal, glow_terminal, {
            "weight": 0.5,
            "weight_explanation": f"LLM error: {exc}",
            "a_score": 0, "b_score": 0,
            "issues": [str(exc)],
            "explanation": f"LLM evaluation error: {exc}",
        })
        return (key, result, action, True)

    result = assemble_result(section, example_num, markdown, html,
                             mdflow_terminal, glow_terminal, llm)
    return (key, result, action, False)


def cmd_run(filters: dict):
    if not SPEC_PATH.exists():
        die(f"spec.json not found: {SPEC_PATH}")
    if not MDFLOW_BIN.exists():
        die(f"mdflow binary not found: {MDFLOW_BIN}")

    api_key = os.environ.get("OPENAI_API_KEY", "")
    if not api_key:
        die("OPENAI_API_KEY environment variable is not set.")

    spec = read_json(SPEC_PATH, [])
    entries = apply_spec_filters(spec, filters)
    if not entries:
        die("No spec entries matched filters.")

    force = filters.get("force", False)
    workers = filters.get("workers", 3)

    sections = sorted(set(e["section"] for e in entries))
    label = ", ".join(sections) if len(sections) <= 3 else f"{len(sections)} sections"
    print(f"Run   {len(entries)} examples  ({label})  "
          f"{'force' if force else 'incremental'}  "
          f"{'parallel' if workers > 1 else 'sequential'}")
    print(f"Model {MODEL}")
    print()

    results = read_json(RESULTS_PATH, [])
    result_map = {(e["section"], e["example"]): e for e in results}

    changed = 0
    skipped = 0
    total = len(entries)
    sequential = workers == 1

    with concurrent.futures.ThreadPoolExecutor(max_workers=max(1, workers)) as pool:
        fut_to_idx = {}
        for i, entry in enumerate(entries):
            fut = pool.submit(_eval_one, api_key, entry, result_map, force)
            fut_to_idx[fut] = i
            if sequential:
                time.sleep(0.15)

        for future in concurrent.futures.as_completed(fut_to_idx):
            i = fut_to_idx[future]
            entry = entries[i]
            try:
                key, result, action, is_error = future.result()
            except Exception as exc:
                print(f"[{i + 1:>3}/{total}] FAIL   #{entry['example']} "
                      f"{entry['section']:<30}  worker error: {exc}")
                continue

            if result is None:
                skipped += 1
                print(f"[{i + 1:>3}/{total}] Skip   #{entry['example']} "
                      f"{entry['section']:<30} (unchanged)")
                continue

            if is_error:
                print(f"[{i + 1:>3}/{total}] FAIL   #{entry['example']} "
                      f"{entry['section']:<30}  {result.get('explanation', 'error')[:80]}")
                continue

            changed += 1
            result_map[key] = result
            print(f"[{i + 1:>3}/{total}] {action:<6} #{entry['example']} "
                  f"{entry['section']:<30}  "
                  f"mdflow={result['mdflow_score']}  "
                  f"glow={result['glow_score']}  "
                  f"weight={result['weight']:.1f}  "
                  f"{_pass_fail(result['pass'])}")
            write_json(RESULTS_PATH, list(result_map.values()))

    write_json(RESULTS_PATH, list(result_map.values()))

    ran_keys = {(e["section"], e["example"]) for e in entries}
    ran = [result_map[k] for k in ran_keys if k in result_map]

    print()
    print(f"Saved  {RESULTS_PATH}  ({len(result_map)} total, "
          f"{changed} updated, {skipped} skipped)")
    _print_run_summary(ran)


def cmd_show(filters: dict):
    results = read_json(RESULTS_PATH, [])

    if not results:
        print("No results yet. Run: eval.py run <section>")
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
            _print_example_detail(entry, raw=filters.get("raw", False))
    elif has_section_filter and len(filters["sections"]) == 1 and not has_verdict_filter:
        _print_section_detail(results, filters["sections"][0])
    elif has_verdict_filter and not has_section_filter and not has_example_filter:
        _print_section_table(results, filtered)
        print()
        label = "pass" if True in filters["verdicts"] else "fail"
        _print_example_list(filtered, title=f"All {label}")
    elif has_section_filter and has_verdict_filter:
        label = (f"{filters['sections'][0]} (fail)"
                 if False in filters["verdicts"]
                 else f"{filters['sections'][0]} (pass)")
        _print_example_list(filtered, title=label)
    elif has_any_filter:
        _print_example_list(filtered, title=f"Matched {len(filtered)} examples")
    else:
        _print_section_table(results, filtered)


def cmd_diff(filters: dict):
    results = read_json(RESULTS_PATH, [])
    if not results:
        print("No results yet. Run: eval.py run <section>")
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
        raw = filters.get("raw", False)
        for d in diffs:
            print()
            print(f"{_bar(70)}")
            print(f"  Example {d['example']}  section={d['section']}  "
                  f"prev_mdflow={d['prev_mdflow_score']}  {_pass_fail(d['prev_pass'])}")
            print(f"{_bar(70)}")
            print()
            print("--- PREVIOUS (eval_results.json) ---")
            if raw:
                print(repr(d["previous"]))
            else:
                print(d["previous"])
            print()
            print("--- CURRENT (mdflow) ---")
            if raw:
                print(repr(d["current"]))
            else:
                print(d["current"])

    _reset_terminal()
    print()
    print(f"Summary  {len(filtered)} entries  "
          f"{same_count} same  {diff_count} diff  {skip_count} skipped")


# ── Table rendering ────────────────────────────────────────────────


def table(headers: list[str], rows: list[list], right: set = None,
          top: list = None, bottom: list = None, note: str = ""):
    right = right or set()
    all_rows = [headers] + rows
    if top:
        all_rows.insert(0, top)
    widths = [max(len(str(r[i])) for r in all_rows) for i in range(len(headers))]

    def render(row):
        return "  ".join(
            str(row[i]).rjust(widths[i]) if i in right else str(row[i]).ljust(widths[i])
            for i in range(len(row))
        )

    if top:
        print(render(top))
    print(render(headers))
    print("  ".join("─" * w for w in widths))
    for row in rows:
        print(render(row))
    if bottom:
        print(render(bottom))
    if note:
        print(note)
    print()


# ── Display helpers ────────────────────────────────────────────────


def die(msg: str):
    print(f"Error: {msg}", file=sys.stderr)
    sys.exit(1)


def _print_run_summary(entries: list):
    passes = sum(1 for e in entries if e.get("pass") is True)
    fails = sum(1 for e in entries if e.get("pass") is False)
    print(f"Summary  {passes} pass  "
          f"{fails} fail")


def _print_section_table(all_results: list, filtered_results: list):
    spec = read_json(SPEC_PATH, [])
    section_order = spec_section_order()

    spec_counts = {}
    for e in spec:
        spec_counts[e["section"]] = spec_counts.get(e["section"], 0) + 1

    result_map = {(e["section"], e["example"]): e for e in all_results}

    norm = _normalized_weighted_avg(all_results)
    section_avgs = norm["sections"]
    global_md, global_gl = norm["global"]

    rows = []
    total_all = total_done = all_p = all_f = 0
    for sec in section_order:
        total = spec_counts.get(sec, 0)
        sec_res = [e for e in all_results if e["section"] == sec]
        done = len(sec_res)
        p = sum(1 for e in sec_res if e.get("pass") is True)
        f = sum(1 for e in sec_res if e.get("pass") is False)
        md = section_avgs.get(sec, (0, 0))[0]
        gl = section_avgs.get(sec, (0, 0))[1]

        d = str(done) if done else "-"
        rows.append([
            str(len(rows) + 1), _trim_name(sec, 12),
            str(total), d,
            str(p) if done else "-", str(f) if done else "-",
            f"{md:.1f}" if done else "-", f"{gl:.1f}" if done else "-",
        ])
        total_all += total
        total_done += done
        all_p += p
        all_f += f

    print()
    print("mdflow AI Test Results")

    headers = ["#", "Section", "Total", "Done", "pass", "fail", "avg", "avg"]
    top_row = ["", "", "", "", "mdflow", "mdflow", "mdflow", "glow"]
    right_idx = {0, 2, 3, 4, 5, 6, 7}

    bottom = None
    if total_done:
        bottom = ["", "Total", str(total_all), str(total_done),
                  str(all_p), str(all_f),
                  f"{global_md:.1f}", f"{global_gl:.1f}"]

    table(headers, rows, right=right_idx, top=top_row, bottom=bottom,
          note="Note: Pass/Fail is weighted. Avg Score is weight-band-normalized.")


def _print_section_detail(all_results: list, section: str):
    spec = read_json(SPEC_PATH, [])
    spec_entries = [e for e in spec if e["section"] == section]
    result_map = {(e["section"], e["example"]): e for e in all_results}
    tested = [result_map[(section, e["example"])] for e in spec_entries
              if (section, e["example"]) in result_map]

    p_count = sum(1 for e in tested if e.get("pass") is True)
    f_count = sum(1 for e in tested if e.get("pass") is False)

    if tested:
        sec_avgs = _normalized_weighted_avg(tested)
        wm_avg, wg_avg = sec_avgs["sections"].get(section, (0, 0))
    else:
        wm_avg = wg_avg = 0.0

    print()
    print(f"Section: {section}  ({len(tested)}/{len(spec_entries)} tested)")

    rows = []
    for e in spec_entries:
        num = e["example"]
        key = (section, num)
        if key in result_map:
            r = result_map[key]
            rows.append([
                f"#{num}",
                f"{r.get('weight', 0):.1f}",
                _pass_fail(r.get("pass", False)),
                str(r.get("mdflow_score", "?")),
                str(r.get("glow_score", "?")),
            ])
        else:
            rows.append([f"#{num}", "-", "-", "-", "-"])

    headers = ["#", "weight", "verdict", "score", "score"]
    top_row = ["", "", "mdflow", "mdflow", "glow"]
    right_idx = {1, 3, 4}

    bottom = None
    if tested:
        bottom = ["", "", "Avg", f"{wm_avg:.1f}", f"{wg_avg:.1f}"]

    table(headers, rows, right=right_idx, top=top_row, bottom=bottom,
          note="Note: Pass/Fail is weighted. Avg Score is weight-band-normalized.")


def _print_example_list(entries: list, title: str = ""):
    if title:
        print()
        print(title)
    if not entries:
        print("  (none)")
        return

    print()
    max_num = max((e.get("example", 0) for e in entries), default=0)
    num_width = len(str(max_num))

    headers = ["#", "weight", "mdflow", "glow", "verdict", "section"]
    rows = []
    for e in entries:
        rows.append([
            f"#{e.get('example', '?'):>{num_width}}",
            f"{e.get('weight', 0):.1f}",
            str(e.get("mdflow_score", "?")),
            str(e.get("glow_score", "?")),
            _pass_fail(e.get("pass")),
            e.get("section", "?"),
        ])

    table(headers, rows, right={0, 1, 2, 3})


def _print_example_detail(entry: dict, raw: bool = False):
    section = entry.get("section", "?")
    example = entry.get("example", "?")
    weight = entry.get("weight", "?")
    weight_expl = entry.get("weight_explanation", "")
    passed = entry.get("pass", "?")
    m_score = entry.get("mdflow_score", "?")
    g_score = entry.get("glow_score", "?")
    issues = entry.get("issues", [])
    explanation = entry.get("explanation", "")
    markdown = entry.get("markdown", "")
    html = entry.get("html", "")
    md_terminal = entry.get("mdflow_terminal", "")
    gl_terminal = entry.get("glow_terminal", "")

    W = 70

    print()
    print(f"Example #{example}  {section}  "
          f"weight={weight:.1f}  {_pass_fail(passed)}" +
          (f"  --raw" if raw else ""))
    print(_bar(W))

    if weight_expl:
        print(f"  {'Weight:'} {weight_expl}")

    print()
    print(f"  {'':<10}  score")
    print(f"  {'Mdflow':<10}  {m_score:>5}/3")
    print(f"  {'Glow':<10}  {g_score:>5}/3")

    if explanation:
        print()
        print("  Explanation:")
        for line in _wrap_text(explanation, W - 4):
            print(f"    {line}")

    if issues:
        print()
        print("  Issues:")
        for issue in issues:
            print(f"    - {issue}")

    if markdown:
        print()
        print(f"  {'─' * (W - 4)}")
        print("  Markdown input:")
        for line in markdown.rstrip("\n").split("\n"):
            print(f"    {line}" if line.strip() else f"    (blank)")

    if html:
        print()
        print(f"  {'─' * (W - 4)}")
        if raw:
            print("  Expected HTML (raw):")
            for line in html.rstrip("\n").split("\n"):
                if len(line) > W - 4:
                    line = line[:W - 7] + "..."
                print(f"    {line}")
        else:
            rendered = _html_to_ansi(html)
            if rendered is not None:
                print("  Expected HTML (rendered via rich):")
                for line in rendered.rstrip("\n").split("\n"):
                    if len(line) > W - 4:
                        line = line[:W - 7] + "..."
                    print(f"    {line}")
            else:
                print("  Expected HTML (render error, showing raw):")
                for line in html.rstrip("\n").split("\n"):
                    if len(line) > W - 4:
                        line = line[:W - 7] + "..."
                    print(f"    {line}")

    if md_terminal:
        print()
        print(f"  {'─' * (W - 4)}")
        if raw:
            print("  Mdflow terminal (raw repr):")
            for line in repr(md_terminal).split("\\n"):
                if len(line) > W - 4:
                    line = line[:W - 7] + "..."
                print(f"    {line}")
        else:
            print("  Mdflow terminal (rendered):")
            for line in md_terminal.rstrip("\n").split("\n"):
                stripped = line.rstrip()
                if len(stripped) > W - 4:
                    stripped = stripped[:W - 7] + "..."
                print(f"    {stripped}")

    if gl_terminal:
        print()
        print(f"  {'─' * (W - 4)}")
        if raw:
            print("  Glow terminal (raw repr):")
            for line in repr(gl_terminal).split("\\n"):
                if len(line) > W - 4:
                    line = line[:W - 7] + "..."
                print(f"    {line}")
        else:
            print("  Glow terminal (rendered):")
            for line in gl_terminal.rstrip("\n").split("\n"):
                stripped = line.rstrip()
                if len(stripped) > W - 4:
                    stripped = stripped[:W - 7] + "..."
                print(f"    {stripped}")
    _reset_terminal()
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
        print(f"  {prog} diff    [filters]   regression check (no AI)", file=sys.stderr)
        print(file=sys.stderr)
        print("Filters:", file=sys.stderr)
        print('  <section>       e.g. Tabs, "ATX headings"', file=sys.stderr)
        print("  <number>        e.g. 23, 1,3,5, 1-10", file=sys.stderr)
        print("  ALL             all entries", file=sys.stderr)
        print("  --pass          only pass=true", file=sys.stderr)
        print("  --fail          only pass=false", file=sys.stderr)
        print("  --force         force re-run (skip unchanged check)", file=sys.stderr)
        print("  --workers=N     parallel workers (default 3, max 10)", file=sys.stderr)
        print(file=sys.stderr)
        print("Output flags (show/diff):", file=sys.stderr)
        print("  --json          machine-readable JSON output", file=sys.stderr)
        print("  --raw           show raw code (repr) instead of rendered terminal", file=sys.stderr)
        print(file=sys.stderr)
        print("Examples:", file=sys.stderr)
        print(f"  {prog} run Tabs               run Tabs examples", file=sys.stderr)
        print(f"  {prog} run 23                  run single example", file=sys.stderr)
        print(f"  {prog} show                    section summary", file=sys.stderr)
        print(f"  {prog} show Tabs               section detail", file=sys.stderr)
        print(f"  {prog} show Tabs --fail        failed Tabs examples", file=sys.stderr)
        print(f"  {prog} show 1                  full example detail (rendered)", file=sys.stderr)
        print(f"  {prog} show 1 --raw            full example detail (raw code)", file=sys.stderr)
        print(f"  {prog} diff --raw              diff with raw repr", file=sys.stderr)
        sys.exit(1)

    command = sys.argv[1]
    raw_args = sys.argv[2:]

    if command not in ("run", "show", "diff"):
        print(f"Unknown command: {command}", file=sys.stderr)
        sys.exit(1)

    filters = parse_filters(raw_args)

    try:
        if command == "run":
            cmd_run(filters)
        elif command == "show":
            cmd_show(filters)
        elif command == "diff":
            cmd_diff(filters)
    finally:
        _reset_terminal()


if __name__ == "__main__":
    main()
