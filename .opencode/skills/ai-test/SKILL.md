---
name: ai-test
description: Reference for test/ai_test.py — the AI-powered evaluation and regression verification harness for mdflow. Use ONLY when running, showing, or verifying AI test results via ai_test.py.
---

# ai_test.py

Usage reference for the mdflow AI test harness. All scripts live in `test/`.

## Files

| File | Role |
|------|------|
| `ai_test.py` | Main harness: `run`, `show`, `diff` |
| `ai_weights.py` | One-time weight computation |
| `ai_prompts.py` | Prompt templates + spec.txt context extraction (imported by the other two) |
| `spec.json` | 652 CommonMark examples (read-only) |
| `weights.json` | Example weights 0.0–1.0 (created by `ai_weights.py`) |
| `ai_results.json` | Evaluation results (created/updated by `run`, read by `show`/`diff`) |
| `glow_clean.json` | Glow theme used as reference renderer |

Example numbering: `spec.json`, `weights.json`, and `ai_results.json` all key on `(section, example)` tuples from the same `spec.json` — no drift.

## Setup

```bash
python3 ai_weights.py ALL          # compute weights for all 652 examples
python3 ai_weights.py Tabs         # single section
python3 ai_weights.py 23           # single example (by number)
```

Supports resume: skips already-computed entries. Saves incrementally. Requires `OPENAI_API_KEY` env var.

## Commands

```bash
cd test && python3 ai_test.py <command> [filters...]
```

### `run` — AI evaluation

Runs `../mdflow` and `glow -s glow_clean.json` on each matching example. Sends both outputs + weight + spec.txt section context to gpt-5-mini. Saves/merges into `ai_results.json`.

```
[N/M] NEW|UPD|SKIP #num section  m=X  g=X  w=X.X  PASS|FAIL
```

- NEW: first time evaluating this example
- UPD: overwriting a previous result
Run output: `[N/M] New|Update|Skip #num section  mdflow=X  glow=X  weight=X.X  Pass|Fail`. Skip shows only `#num section (unchanged)` — no scores.

Requires `weights.json` (run `ai_weights.py` first). Also requires `../mdflow` binary (`make build`) and `glow` on PATH. Requires `OPENAI_API_KEY`.

### `show` — display results

No AI. Detail level depends on filters:

| Invocation | What it shows |
|------------|---------------|
| `show` (no filters) | All 26 sections table: total/done/pass/fail/weighted avg for both renderers |
| `show <section>` | Section detail: every example with `weight=X.X mdflow=X glow=X Pass/Fail` |
| `show <num>` or `<num>,<num>` | Full detail per example: weight explanation, score breakdown, issues, explanation, markdown, HTML, both terminal outputs |

Weighted avg: examples are bucketed into 10 weight bands (0.1–1.0). Within each section, every band that has data gets one equal vote — a single w=1.0 example does not dominate 100 w=0.1 examples. Per-section avg is the mean of band averages. Global avg is the mean of section averages (each section votes equally).

### `diff` — regression check

No AI. Re-runs `../mdflow` on each result's saved markdown, diffs stdout against saved `mdflow_terminal`.

```
SAME  #num section      (output unchanged)
DIFF  #num section      (output changed — prints both repr()s)
SKIP  #num section      (no saved terminal output)
```

Does NOT modify `ai_results.json`. Use to check if code changes broke previously-evaluated examples.

## Filters

Apply to all three commands:

| Filter | Matches |
|--------|---------|
| `Tabs`, `"ATX headings"` | All examples in that section |
| `23` | Single example number |
| `1,3,5`, `1-10` | Specific or ranged examples |
| `ALL` | All 652 examples |
| `--pass` | Only pass=true |
| `--fail` | Only pass=false |
| `--json` | Machine-readable JSON output (show/diff only) |

Combine freely: `Tabs --fail`, `"ATX headings" 1,3,5`.

## Result format

Each entry in `ai_results.json`:

```json
{
  "section": "Tabs",
  "example": 1,
  "markdown": "...",
  "html": "...",
  "weight": 0.6,
  "weight_explanation": "...",
  "mdflow_terminal": "...",
  "mdflow_content": 1, "mdflow_structure": 1, "mdflow_style": 1,
  "mdflow_score": 3,
  "glow_terminal": "...",
  "glow_content": 1, "glow_structure": 1, "glow_style": 0,
  "glow_score": 2,
  "pass": true,
  "issues": ["..."],
  "explanation": "..."
}
```

Scoring: content + structure + style (0-1 each) = 0–3 per renderer. Content is the foundation — structure and style build on it. Pass/fail is for mdflow only, weight-aware, decided by the AI.

## Prerequisites

- `make build` from project root (requires `../mdflow` binary)
- `glow` on PATH (Renderer B reference)
- `python3` with `openai` package installed
- `OPENAI_API_KEY` env var for `run` and `ai_weights.py`
