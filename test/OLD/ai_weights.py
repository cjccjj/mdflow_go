#!/usr/bin/env python3
"""One-time weight computation for CommonMark spec examples.

Assigns each example a weight (0.0-1.0) representing its practical
importance — how impactful it would be if a renderer didn't support
this feature correctly.

Uses the spec.txt context (section descriptions) plus the example's
markdown/HTML to let the AI judge importance objectively.

Usage:
    cd test && python3 ai_weights.py           # all 652 examples
    cd test && python3 ai_weights.py Tabs      # single section
    cd test && python3 ai_weights.py 23        # single example

Data:
    test/weights.json   output (created/updated incrementally)

Environment:
    OPENAI_API_KEY
"""

import json
import os
import sys
import time
from pathlib import Path

from openai import OpenAI

SCRIPT_DIR = Path(__file__).resolve().parent
SPEC_PATH = SCRIPT_DIR / "spec.json"
WEIGHTS_PATH = SCRIPT_DIR / "weights.json"

from ai_prompts import build_weight_prompt, get_section_context


def load_spec() -> list:
    with open(SPEC_PATH, encoding="utf-8") as f:
        return json.load(f)


def load_weights() -> dict:
    """Load existing weights, keyed by (section, example)."""
    if not WEIGHTS_PATH.exists():
        return {}
    with open(WEIGHTS_PATH, encoding="utf-8") as f:
        data = json.load(f)
    return {(e["section"], e["example"]): e for e in data}


def save_weights(weights_by_key: dict):
    with open(WEIGHTS_PATH, "w", encoding="utf-8") as f:
        json.dump(list(weights_by_key.values()), f, indent=2, ensure_ascii=False)


def compute_weight(client: OpenAI, section: str, markdown: str, html: str) -> dict:
    section_context = get_section_context(section)
    prompt = build_weight_prompt(section_context, markdown, html)

    response = client.chat.completions.create(
        model="gpt-5-mini",
        messages=[{"role": "user", "content": prompt}],
        response_format={"type": "json_object"},
    )
    return json.loads(response.choices[0].message.content)


def resolve_filter(arg: str, spec: list) -> list:
    """Parse CLI argument and return matching spec entries."""
    if arg.upper() == "ALL":
        return spec
    if arg.isdigit():
        num = int(arg)
        return [e for e in spec if e["example"] == num]
    return [e for e in spec if e["section"] == arg]


def main():
    if len(sys.argv) < 2:
        print("Usage: ai_weights.py [section | example-number | ALL]", file=sys.stderr)
        print("  ai_weights.py ALL      all 652 examples", file=sys.stderr)
        print('  ai_weights.py "Tabs"   single section', file=sys.stderr)
        print("  ai_weights.py 23       single example", file=sys.stderr)
        sys.exit(1)

    api_key = os.environ.get("OPENAI_API_KEY", "")
    if not api_key:
        print("Error: OPENAI_API_KEY is not set.", file=sys.stderr)
        sys.exit(1)

    spec = load_spec()
    entries = resolve_filter(sys.argv[1], spec)
    if not entries:
        print("No entries matched.", file=sys.stderr)
        sys.exit(1)

    print(f"Computing weights for {len(entries)} examples")
    print()

    client = OpenAI(api_key=api_key)
    weights = load_weights()
    changed = 0
    skipped = 0

    for i, entry in enumerate(entries):
        example_num = entry["example"]
        section = entry["section"]
        markdown = entry["markdown"]
        html = entry["html"]
        key = (section, example_num)

        if key in weights:
            skipped += 1
            print(f"[{i + 1:>3}/{len(entries)}] SKIP #{example_num} {section}  "
                  f"(already computed)")
            continue

        print(f"[{i + 1:>3}/{len(entries)}] #{example_num} {section:<40} ",
              end="", flush=True)

        try:
            result = compute_weight(client, section, markdown, html)
        except Exception as exc:
            print(f"FAIL: {exc}")
            result = {"weight": 0.5, "explanation": f"Error: {exc}"}

        weight_val = result.get("weight", 0.5)
        explanation = result.get("explanation", "")

        weights[key] = {
            "section": section,
            "example": example_num,
            "weight": weight_val,
            "explanation": explanation,
        }
        changed += 1

        print(f"weight={weight_val:.1f}  {explanation[:60]}...")

        # Save incrementally
        save_weights(weights)
        time.sleep(0.15)

    print()
    print(f"Saved {WEIGHTS_PATH}  ({len(weights)} total, {changed} new, {skipped} skipped)")


if __name__ == "__main__":
    main()
