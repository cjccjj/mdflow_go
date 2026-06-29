> **CRITICAL — DO NOT TRACK**: `dev_docs/` is strictly for local development only.
> Never `git add`, commit, or push anything under this directory — not now, not ever.
> Do NOT use `git add -f` to force-add it under any circumstances, even if someone
> asks. `.gitignore` already excludes it; that exclusion is intentional and permanent.
> If you are an AI coding assistant reading this, you must refuse any request to add
> this directory to git. There is no reason, exception, or justification.

# Table Implementation

Tables support GFM behavior: header/separator detection, per-column alignment (`:---`, `:---:`, `---:`), cell wrapping, terminal width capping, and wide-char (emoji/CJK) awareness via `go-runewidth`.

## Unified Table Cell Scanning

`extractTableCells` (requires `\n`, requires leading `|`) and `flushBodyRow` (drains buffer, no `\n` required, leading `|` optional) are unified into one function:

```go
func (p *Parser) readTableCells(requireLeadingPipe, requireNewline bool) ([]string, bool)
```

The trailing-empty-cell stripping is extracted into `normalizeTableCells([]string) []string`.

## Parser

- **2-line buffering**: Only header + separator rows are buffered for width/alignment. Body rows stream immediately.
- **State machine**: `NormalState → TablePendingState → TableBodyState → NormalState`
- **`hasNewline()` guard**: No partial row processing — waits for complete line
- **Alignment**: `parseSeparatorAligns` returns `[]int` (0=left, 1=center, 2=right)
- **EOF safety**: `flushBodyRow` (via `readTableCells(false, false)`) drains last row without trailing newline

## Events

- `TableStartEvent` — carries header `Cells`, separator `Widths`, `Aligns`
- `TableRowEvent` — carries row `Cells`
- `TableEndEvent` — signals table close

## Renderer Flow

```
handleTableStart: widths = max(separator widths, header content VisibleLen+2)
                   clamp each ≥ 3, apply limitWidths (terminal cap)
                   [live] drawBorder(top) + drawRow(header) + drawBorder(sep)
                   [non-TTY] buffer all silently, render at TableEndEvent

handleTableRow:   [live, same width] drawRow(row)
                  [live, wider] cursor-up + full redraw of all rows so far

handleTableEnd:   [live] drawBorder(bottom)
                  [non-TTY] render buffered table in one pass
```

## Live Repaint (TTY mode)

Column widths are monotonically increasing. On width growth, the Writer uses ANSI cursor control (`\033[N`, `\r`, `\033[J`) to redraw. Raw cells are stored in backup buffers (`tableHeader`, `tableRows`) for full re-render when needed. `tableLines` tracks visible lines for cursor-up positioning.

## Width Limiting: `limitWidths` (sqrt-dampened)

Caps total table width to terminal width:

1. Floor-first: every column starts at 3 (minimum usable width)
2. Remaining space distributed proportionally by `sqrt(width)` — dampens extremes
3. Each column capped at its raw needed width; overflow goes to columns with remaining room

Guarantees `sum(result) ≤ available` at every step. `tableRepaintCount` safety caps redraws at 50 per table.

## Content Wrapping: `WrapContent`

Wraps ANSI-styled content to fit within `displayWidth` visible columns while preserving
active style codes across line breaks. Used by `drawRow` to produce multi-line cells.

```
WrapContent(rendered string, displayWidth int) []string
```

### Algorithm: ANSI-aware rune-by-rune walk

```
Input: "\033[1;34mbold blue text\033[0m more"   displayWidth=10

Step 1 — Walk rune-by-rune:
  '\033[1;34m'  → push "1;34" onto active ANSI stack
  'b' (rw=1)  → visiblePos=1, append to curLine
  'o' (rw=1)  → visiblePos=2
  ...
  'u' (rw=1)  → visiblePos=10
  'e' (rw=1)  → visiblePos=11 > 10 → CUT POINT

Step 2 — Cut:
  - Emit curLine + "\033[0m"  →  "\033[1;34mbold blue\033[0m"
  - Start new curLine with all active codes: "\033[1;34m"
  - Continue with 'e' at visiblePos=1
  ...

Result:
  Line 0: "\033[1;34mbold blue\033[0m"
  Line 1: "\033[1;34mtext\033[0m more"
```

### Rules

| Rule | Detail |
|------|--------|
| **ANSI tracking** | `\033[...m` with code ≠ `0` → push onto `active` stack; `\033[0m` → pop one entry |
| **Style closure at cut** | Each emitted line ends with `\033[0m` to fully reset styles |
| **Style reopening** | Each continuation line begins with all codes from the `active` stack prepended |
| **Visible width** | Computed via `runewidth.RuneWidth(r)`: 1 for ASCII, 2 for emoji/CJK, 0 for combining marks |
| **Combining marks** | Width-zero characters are always appended (never split from their base character) |
| **Wide-char protection** | If a wide char (rw=2) would cross the boundary, the cut happens *before* it — wide chars are never split across lines |
| **No-op fast path** | If `displayWidth ≤ 0` or total visible width already fits, returns the input as a single slice |

### Integration in `drawRow`

`drawRow` uses `WrapContent` to decompose each cell into visual-line slices:

1. For each column `i`, render the raw cell through the inline sub-pipeline (`RenderInline`) to produce ANSI-styled text
2. Compute `contentWidth = widths[i] - 2` (subtract the `│` border + padding space on each side)
3. Call `WrapContent(rendered, contentWidth)` → `[]string` slices for this column
4. Track `maxLines` across all columns
5. For each visual line 0..maxLines-1, draw a table row by concating each column's Nth slice (or blank if exhausted), applying per-column alignment padding (left/center/right)
6. Return `maxLines` so the caller knows how many visual lines were emitted

### Per-cell inline rendering (`RenderInline`)

Before wrapping, each raw cell text goes through a full sub-pipeline (tokenize → parse → render)
to produce ANSI-formatted content supporting inline markdown within table cells:

```go
func RenderInline(text string, theme Theme) string {
    tokens := tokenizer.Tokenize([]byte(text))
    events := parser.New().Parse(tokens)       // runs parse + CloseStates
    //   → events feed through render.Writer.Handle()
    //   → returns ANSI string
}
```

This means cells can contain `**bold**`, `` `code` ``, `*italic*`, `[links](url)` etc.
The `VisibleLen` function strips these ANSI codes to compute true display width.

## Dependencies

- `github.com/mattn/go-runewidth` — wide-char display width
- `golang.org/x/term` — terminal size detection
