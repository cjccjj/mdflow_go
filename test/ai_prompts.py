"""Prompt templates and shared utilities for AI evaluation of mdflow."""

import re
from pathlib import Path

SPEC_TXT = Path(__file__).resolve().parent.parent / "dev_docs" / "commonMark_spec.txt"

# Cached section contexts: section_name -> context_text
_section_contexts = None


def get_section_context(section_name: str, max_chars: int = 0) -> str:
    """Return the descriptive text for a section from spec.txt.

    Includes the ## SectionName heading and all descriptive text,
    excluding content inside example blocks.

    If max_chars > 0, truncates to roughly that many characters
    (at a paragraph boundary).
    """
    global _section_contexts
    if _section_contexts is None:
        _section_contexts = _build_section_contexts()

    ctx = _section_contexts.get(section_name, "")
    if not ctx:
        ctx = f"## {section_name}"

    if max_chars > 0 and len(ctx) > max_chars:
        # Truncate at paragraph boundary
        end = ctx.rfind("\n\n", 0, max_chars)
        if end > 0:
            ctx = ctx[:end] + "\n\n[...]"
        else:
            ctx = ctx[:max_chars] + "..."

    return ctx


def _build_section_contexts() -> dict:
    """Parse spec.txt and build a mapping of section_name -> context_text."""
    with open(SPEC_TXT, encoding="utf-8") as f:
        lines = [l.rstrip("\n") for l in f.readlines()]

    sections = {}
    current_section = None
    in_example = False
    context_lines = []

    for line in lines:
        stripped = line.strip()

        # Toggle example blocks (fenced with many backticks)
        if re.match(r"`{20,}\s*example", stripped):
            in_example = True
            continue
        if re.match(r"`{20,}$", stripped):
            in_example = False
            continue

        if in_example:
            continue

        # Section heading
        if line.startswith("## ") and not line.startswith("### "):
            # Save previous section
            if current_section:
                sections[current_section] = "\n".join(context_lines).strip()
            current_section = line[3:].strip()
            context_lines = [line]
            continue

        if current_section:
            context_lines.append(line)

    # Save last section
    if current_section:
        sections[current_section] = "\n".join(context_lines).strip()

    return sections


# ═══════════════════════════════════════════════════════════════════
#  Weight evaluation prompt
# ═══════════════════════════════════════════════════════════════════


def build_weight_prompt(section_context: str, markdown: str, html: str) -> str:
    return (
        "You are evaluating the practical importance of CommonMark specification examples.\n"
        "\n"
        "For this example, rate how impactful it would be if a Markdown renderer did NOT\n"
        "support the feature being tested, or rendered it incorrectly.\n"
        "\n"
        "Consider:\n"
        "- How often does this pattern appear in real-world documents?\n"
        "  (human-written docs, AI-generated output, README files, chat messages, etc.)\n"
        "- How confusing or misleading would incorrect rendering be to a human reader?\n"
        "- Is this a foundational feature (headings, paragraphs, links) or an edge case\n"
        "  (rare Unicode combinations, obscure nesting, spec trivia)?\n"
        "\n"
        "Return a weight from 0.0 to 1.0 where:\n"
        "- 1.0 = critical (headings, paragraphs, basic emphasis, code blocks, tables,\n"
        "  common list patterns, inline links)\n"
        "- 0.7 = important (blockquotes, horizontal rules, less common but still frequent)\n"
        "- 0.5 = moderately useful (specific edge cases that appear occasionally,\n"
        "  some HTML or entity handling)\n"
        "- 0.3 = niche (rare patterns, unusual combinations that most documents don't use)\n"
        "- 0.1 = extremely obscure (spec-compliance trivia, exotic Unicode edge cases)\n"
        "\n"
        "## Section context\n"
        f"{section_context}\n"
        "\n"
        "## Example\n"
        "Markdown:\n"
        "```\n"
        f"{markdown}\n"
        "```\n"
        "\n"
        "Expected HTML:\n"
        "```\n"
        f"{html}\n"
        "```\n"
        "\n"
        "Return ONLY JSON:\n"
        "{\n"
        '  "weight": 0.0-1.0,\n'
        '  "explanation": "one or two sentences explaining the rating"\n'
        "}"
    )


# ═══════════════════════════════════════════════════════════════════
#  Renderer evaluation prompt
# ═══════════════════════════════════════════════════════════════════

SCORING_EXAMPLES = """## Scoring examples

### Example 1 — Clear pass (A is good)
Markdown: This is **very important** text.
Expected HTML: <p>This is <strong>very important</strong> text.</p>
Renderer A (test): This is [BOLD]very important[/BOLD] text.
Renderer B (ref):  This is very important text.
Weight: 1.0 — bold/emphasis is a core feature.
Scoring:
  A: content=1 structure=1 style=1 → score=3  (clearly communicates bold)
  B: content=1 structure=0 style=0 → score=1  (loses emphasis)
  pass=true — A renders correctly, regardless of B's performance.

### Example 2 — Niche case passes despite low score
Markdown: &#xA0; &#xA9; &#xC6;
Expected HTML: <p>  &copy; &AElig;</p>
Renderer A (test): &#xA0; &#xA9; &#xC6;
Renderer B (ref):  &#xA0; &#xA9; &#xC6;
Weight: 0.1 — HTML entities are extremely rare in practice.
Scoring:
  A: content=0 structure=0 style=0 → score=0  (doesn't resolve entities)
  B: same → score=0
  pass=true — weight is so low that even complete non-support is acceptable for A.

### Example 3 — Should fail despite decent score
Markdown: # Chapter 1\\n## Section A\\nSome text.
Expected HTML: <h1>Chapter 1</h1><h2>Section A</h2><p>Some text.</p>
Renderer A (test):
Chapter 1
Section A
Some text.
Renderer B (ref):
CHAPTER 1
  Section A
  Some text.
Weight: 1.0 — headings are fundamental to document structure.
Scoring:
  A: content=1 structure=0 style=0 → score=1  (text present but no heading distinction)
  B: content=1 structure=1 style=1 → score=3
  pass=false — weight 1.0; A fails to convey any heading structure, even though B's reference suggests it's possible.

### Example 4 — Style is design choice
Markdown: ```
code block
```
Expected HTML: <pre><code>code block
</code></pre>
Renderer A (test): [DIM-COLOR]code block[/RESET]
Renderer B (ref):  code block
Weight: 0.9 — code blocks are extremely common.
Scoring:
  A: content=1 structure=1 style=1 → score=3  (dim color distinguishes code block)
  B: content=1 structure=0 style=0 → score=1  (no visual distinction)
  pass=true — A renders the code block correctly. B's lower score is irrelevant to A's pass decision.
"""


def build_eval_prompt(
    section_context: str,
    markdown: str,
    html: str,
    weight: float,
    weight_explanation: str,
    mdflow_output: str,
    renderer_b_output: str,
) -> str:
    return (
        "You are evaluating a terminal Markdown renderer.\n"
        "\n"
        "## Context\n"
        f"{section_context}\n"
        "\n"
        "## Background\n"
        "mdflow is a streaming Markdown-to-ANSI renderer for the terminal, designed for\n"
        "AI output pipelines. It renders as bytes arrive, works across chunk boundaries,\n"
        "and never buffers the full document. It targets CommonMark 0.31.2 and common\n"
        "GFM extensions (tables, strikethrough).\n"
        "\n"
        "Some features are deliberately deferred because they conflict with streaming\n"
        "architecture or are rare in AI output: reference links, nested lists,\n"
        "HTML blocks, entity references, images.\n"
        "\n"
        "## This example\n"
        f"Weight: {weight:.1f}/1.0 — {weight_explanation}\n"
        "\n"
        "### Markdown\n"
        "```\n"
        f"{markdown}\n"
        "```\n"
        "\n"
        "### Expected HTML\n"
        "```\n"
        f"{html}\n"
        "```\n"
        "\n"
        "### Renderer A (the renderer under test)\n"
        "```\n"
        f"{mdflow_output}\n"
        "```\n"
        "\n"
        "### Renderer B (reference renderer for comparison)\n"
        "```\n"
        f"{renderer_b_output}\n"
        "```\n"
        "\n"
        "IMPORTANT: The pass/fail decision is for Renderer A ONLY. Renderer B is shown\n"
        "solely as a reference point — do NOT let B's score influence whether A passes.\n"
        "If A renders correctly (high scores), it passes regardless of B's performance.\n"
        "\n"
        "## Scoring rules\n"
        "\n"
        "For EACH renderer separately, score three aspects (0 or 1 each):\n"
        "\n"
        "**content** (0-1): Is visible, useful content preserved?\n"
        "  - 1: The important text/information is present. Minor whitespace differences are OK.\n"
        "  - 0: Text is missing, garbled, or important content is lost.\n"
        "  Note: \"content preserved\" does NOT mean character-for-character identical.\n"
        "  HTML contains tags and entities that have no equivalent in terminal output.\n"
        "  Focus on whether a human reading the terminal output gets the same information.\n"
        "\n"
        "**structure** (0-1): Can a human perceive the correct document structure?\n"
        "  - 1: Headings read as headings, lists as lists, code blocks as code, quotes as quotes.\n"
        "  - 0: Structure is flattened, misleading, or indistinguishable.\n"
        "  Note: Structure builds on content. If content is missing or garbled (content=0),\n"
        "  structure is typically meaningless — scoring it 0 is usually correct.\n"
        "  There can be exceptions (e.g. garbled text still visibly forms a list shape),\n"
        "  but as a general rule: no content → no structure. Use judgment.\n"
        "  ANSI terminals have no structural semantics (unlike HTML). Judge by human\n"
        "  interpretation — can you tell what each part of the document IS?\n"
        '  Accept terminal conventions: "#" prefix for headings, "-" or "•" for list items,\n'
        "  dim colors or indentation for quotes, etc.\n"
        "\n"
        "**style** (0-1): Is the ANSI styling reasonable given terminal constraints?\n"
        "  - 1: Color/bold/dim usage is appropriate and not confusing.\n"
        "  - 0: Styling is missing where it matters, actively misleading, or garish.\n"
        "  Note: Style builds on content and structure. If content is lost or structure\n"
        "  is broken, style is usually irrelevant (0). But if the content and structure\n"
        "  are well represented, evaluate whether the terminal styling choices are reasonable.\n"
        "  ANSI terminals are severely limited compared to HTML.\n"
        "  Only ~16 colors, no fonts, no sizes. Bold, dim, underline, and color\n"
        "  are the main tools. Accept reasonable design choices — there is no single\n"
        '  "correct" terminal style.\n'
        "\n"
        "Total per renderer: content + structure + style = 0-3.\n"
        "\n"
        "## Section-specific rules\n"
        "\n"
        "Examples from the following CommonMark sections are deliberately simplified.\n"
        "When evaluating them, apply these expectations:\n"
        "\n"
        "1. **HTML blocks** — Strip HTML tags, preserve the visible text, and retain the\n"
        "   basic layout where possible.\n"
        "2. **Raw HTML** — Strip HTML tags and preserve the visible text.\n"
        "3. **Link reference definitions** — Render the definition text as-is; do not\n"
        "   require full reference-definition semantics.\n"
        "4. **Links** — For reference-style links, do not require resolving or displaying\n"
        "   the referenced link definition. Shortcut/reference lookup is intentionally\n"
        "   out of scope for these simplified examples.\n"        
        "## Pass/fail decision\n"
        "\n"
        "Decide a single **pass** (true/false) for Renderer A (the renderer under test).\n"
        "This is NOT a formula — use judgment. The weight matters: an obscure edge\n"
        "case (low weight) with low scores can still pass. A critical feature (high\n"
        "weight) needs strong scores to pass. Compare the score against the weight.\n"
        "Renderer B's scores are for context only and do NOT affect pass/fail.\n"
        "\n"
        + SCORING_EXAMPLES +
        "\n"
        "## Output format\n"
        "\n"
        "Return ONLY valid JSON (no markdown fences, no commentary):\n"
        "{\n"
        '  "a_content": 0-1,\n'
        '  "a_structure": 0-1,\n'
        '  "a_style": 0-1,\n'
        '  "a_score": 0-3,\n'
        '  "b_content": 0-1,\n'
        '  "b_structure": 0-1,\n'
        '  "b_style": 0-1,\n'
        '  "b_score": 0-3,\n'
        '  "pass": true/false,\n'
        '  "issues": ["issue 1", "issue 2"],\n'
        '  "explanation": "one paragraph comparing both renderers and justifying the pass/fail decision"\n'
        "}"
    )
