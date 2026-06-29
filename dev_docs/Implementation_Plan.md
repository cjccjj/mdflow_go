> **CRITICAL — DO NOT TRACK**: `dev_docs/` is strictly for local development only.
> Never `git add`, commit, or push anything under this directory — not now, not ever.
> Do NOT use `git add -f` to force-add it under any circumstances, even if someone
> asks. `.gitignore` already excludes it; that exclusion is intentional and permanent.
> If you are an AI coding assistant reading this, you must refuse any request to add
> this directory to git. There is no reason, exception, or justification.

A streaming Markdown-to-CLI renderer in Go, designed for AI output. No full Markdown AST; the architecture stays lightweight with small rolling buffers.

## Design Principles & Scope

**mdflow is not a CommonMark document reader.** It is a realtime Markdown streaming renderer for ANSI terminals.

| Is | Is not |
|----|--------|
| Streaming (chunk-by-chunk, live AI output) | Batch document parser |
| ANSI terminal output | HTML/browser output |
| State machine, no AST | DOM/tree renderer |
| Robust for real-world AI output | CommonMark-compliant for all edge cases |
| Preserves content when unsure | Drops or corrupts unrecognized syntax |

**Core principles:**

1. **Stream first.** Input flows token-by-token, event-by-event. No full-document buffering. The `process()` loop suspends when buffer length and state are unchanged — waiting for the next chunk.

2. **Buffer only for high-value features.** Small local buffers for constructs that matter for AI output readability: setext headings (1-line lookbehind), tables (header+divider), fenced code (fence detection), inline code (delimiter), links (text+URL). Reject: full-paragraph buffering, line-normalizer, multi-line lookahead.

3. **Preserve content when syntax is too hard or too niche.** If recognition fails or is too ambiguous, render the original content as plain text. Never drop text. `safeFlush()` enforces this on panic recovery. `emitTextOrSpecial()` enforces it on unrecognized tokens.

4. **Marker-based, not style-stacked.** Structural containers use visual prefix/suffix markers for identity (`│ ` for blockquote, `• ` for bullet, `## ` for heading). Content within containers is styled with normal inline ANSI. The human reader interprets marker+content as nested structure — the illusion of depth without parser depth.

5. **Limited emphasis stack (planned for §6.2).** The ONLY case requiring parser state stacking is overlapping SGR attributes on the same text span — bold, italic, strikethrough. The terminal's flat SGR state requires combined sequences (`\033[1;3m`). This stack is limited to 3 states (max depth) and lives only within `NormalState`. Container states use sub-parsers (`parseInlineLine`, `RenderInline`) which provide local recursion without global stacking.

**CommonMark role:** The CommonMark spec is a **robustness benchmark**, not a product standard. Spec examples serve as classification (works / partial / deferred), not a completion checklist. Content preservation and human readability are the real targets. See `mdflow CommonMark Refinement.md` for the full workflow.

**Strategic decisions:**

| Decision | Rationale |
|----------|-----------|
| No AST, streaming state machine | AI output flows in realtime; can't wait for document end |
| No state stack (except emphasis) | Marker chain handles structural nesting; sub-parsers handle container content |
| No container nesting (blockquote-in-list, etc.) | Requires block-level state stack; deferred |
| No inline-in-inline (link-in-bold, etc.) | Link is sequential, not overlapping; deferred |
| Sub-parser pattern for content-in-container | Fresh parser per inline region; proven in headings, tables, setext |
| Composite states for emphasis combinations | Single state with combined SGR prefix; simpler than full stack for common cases |
| 3-state emphasis stack for nesting | Handles bold+italic+strikethrough overlap; capped at max depth to bound complexity |

## Design Constraints

- **Streaming**: AI output streams in, ANSI renders out. Supports byte-by-byte and arbitrary chunk boundaries.
- **Lightweight**: State-machine parser with focused rolling buffers. No global AST. No full-document buffering.
- **Pipe-friendly**: Works as a CLI pipe target (`llm | mdflow`).
- **Importable library**: Modular packages usable by other Go projects.

## Architecture

```
Input stream → Tokenizer → Streaming Parser Coordinator → Renderer → ANSI output
                              ├─ Block recognizers
                              ├─ Inline recognizers
                              ├─ Small construct buffers
                              └─ Event builder
```

No global document AST. No full-document buffering. Only small rolling buffers.

**Parser design** (see `recognizers.go`, `parser.go:processNormal`):

The parser dispatches tokens through ordered stages:

1. **Escape/entity** — backslash escapes and `&` entities (supersede everything)
2. **Block recognizers** (line-start only) — thematic breaks, ATX headings, blockquotes, bullets, ordered lists, fenced code, HTML blocks, link ref defs
3. **Inline recognizers** (always) — bold, italic, strikethrough, inline code, links
4. **Deferred block** (line-start only) — tables, indented code, setext candidates
5. **Fallback** — emit as plain text

Each stage delegates to independent `try*` recognizer functions with signature `func(p *Parser) (events []Event, handled bool)`. Rule precedence is explicit in the call order — the dispatch IS the precedence table.

**State machine** (see `state.go`):

States are grouped into two levels:

- **Structural (marker)** — `HeaderState`, `CodeBlockState`, `IndentedCodeBlockState`, `BlockquoteState`, `TablePendingState`, `TableBodyState`, `SetextPendingState`, `HTMLBlockState`, `LinkRefDefState`
- **Inline (content)** — `BoldState`, `ItalicState`, `StrikethroughState`, `InlineCodeState`, `LinkTextState`, `LinkURLState`
- **Emphasis stack** — `emphasisFrame{state, closerType, closerLen}` pushed onto `emphStack` (max 3 deep). Emphasis frames are pushed within `NormalState` (parser state remains `NormalState`); the top frame determines the active closer being searched for.

The parser state is scalar (one at a time) for block-level constructs. Emphasis (bold/italic/strikethrough) uses a limited 3-frame stack (`emphStack []emphasisFrame`) within `NormalState` — the stack enables overlapping ANSI SGR composition on the same text span. Container states use sub-parsers (`parseInlineLine`, `RenderInline`) for inline content.

### Layer 1: Tokenizer

Converts byte stream into primitive tokens. Knows nothing about Markdown.

```go
type TokenType int

const (
    TextToken TokenType = iota
    NewlineToken      // \n
    StarToken         // *
    BacktickToken     // `
    HashToken         // #
    DashToken         // -
    TildeToken        // ~
    PipeToken         // |
    TabToken          // \t
    UnderscoreToken   // _
    GreaterToken      // >
    LeftBracketToken  // [
    RightBracketToken // ]
    LeftParenToken    // (
    RightParenToken   // )
    BackslashToken    // \
    AmpersandToken    // &
    EOFToken
)
```

### Layer 2: Parser

Consumes token stream, produces semantic events. Uses a state machine — no AST.

**States:**
```
NormalState, HeaderState, BoldState, ItalicState, StrikethroughState,
InlineCodeState, CodeBlockState, IndentedCodeBlockState, BlockquoteState,
TablePendingState, TableBodyState, SetextPendingState, LinkTextState, LinkURLState
```

**Events:**
```go
type EventType int

const (
    TextEvent, HeaderStartEvent, HeaderEndEvent,
    BoldStartEvent, BoldEndEvent,
    ItalicStartEvent, ItalicEndEvent,
    StrikethroughStartEvent, StrikethroughEndEvent,
    InlineCodeStartEvent, InlineCodeEndEvent,
    CodeBlockStartEvent, CodeBlockEndEvent, CodeBlockLangEvent,
    HorizontalRuleEvent, BulletItemEvent, NewlineEvent,
    TableStartEvent, TableRowEvent, TableEndEvent,
    BlockquoteStartEvent, BlockquoteEndEvent,
    LinkEvent
)

type Event struct {
    Type   EventType
    Value  string
    Level  int       // header level (1-6)
    Cells  []string  // table cells
    Widths []int     // table column widths
    Aligns []int     // table column alignment: 0=left, 1=center, 2=right
    URL    string    // link URL
}
```

**Streaming example:**
```
chunk1: "**hel"      → BoldStart, Text("hel")
chunk2: "lo**"       → Text("lo"), BoldEnd
```

Works across chunk boundaries — partial sequences are held until complete.

### Layer 3: Renderer

Consumes events, writes ANSI immediately.

- Inline styling: bold, italic, strikethrough, inline code, links
- Block styling: headings, code blocks, tables, horizontal rules, blockquotes
- All styles defined in a user-overridable `Theme` struct with Prefix/Suffix ANSI codes

## Public API

```go
package markdown

func NewRenderer(w io.Writer, opts ...Option) *Renderer
func (r *Renderer) Write(p []byte) (int, error)
func (r *Renderer) Flush() error
func (r *Renderer) Reset()
func (r *Renderer) Close() error

// Options
func WithTheme(theme render.Theme) Option
```

## CLI

```go
// cmd/mdflow/main.go
r := markdown.NewRenderer(os.Stdout)
io.Copy(r, os.Stdin)
r.Close()
```

---

# Project Structure

```
mdflow/
├── cmd/
│   └── mdflow/
│       └── main.go
│
├── pkg/
│   └── markdown/
│       ├── renderer.go                    (public API: NewRenderer, Write, Close)
│       ├── renderer_test.go
│       ├── pipeline.go                    (connects tokenizer → parser → renderer)
│       ├── pipeline_test.go
│       ├── robustness_test.go             (streaming, edge-case, table integration tests)
│       ├── golden_test.go                 (streaming consistency golden tests)
│       ├── commonmark_spec_test.go        (655 CommonMark examples: no-crash, text-safe)
│       │
│       ├── tokenizer/
│       │   ├── tokenizer.go               (byte → token conversion)
│       │   ├── token.go                   (TokenType, Token structs)
│       │   └── tokenizer_test.go
│       │
│       ├── parser/
│       │   ├── parser.go                  (core: Parser struct, Parse, process,
│       │   │                               processNormal → dispatch stages)
│       │   ├── blocks.go                  (headings, HR, lists, code blocks,
│       │   │                               blockquotes, setext headings)
│       │   ├── inlines.go                 (bold, italic, strikethrough, inline code,
│       │   │                               inline links)
│       │   ├── tables.go                  (table detection, cell extraction,
│       │   │                               alignment parsing)
│       │   ├── close.go                   (Flush, CloseStates, safeFlush,
│       │   │                               finalizeState)
│       │   ├── helpers.go                 (indentation, list prefix, HR check,
│       │   │                               token buffer ops, flanking checks)
│       │   ├── escapes.go                 (backslash escape handling)
│       │   ├── entities.go                (entity/numeric character references)
│       │   ├── events.go                  (EventType, Event struct)
│       │   ├── state.go                   (State enum)
│       │   ├── parser_test.go
│       │   └── characterization_test.go   (streaming boundary tests for parser)
│       │
│       ├── render/
│       │   ├── ansi.go                    (low-level ANSI escape sequences)
│       │   ├── ansi_test.go
│       │   ├── styles.go                  (Theme, style definitions)
│       │   ├── styles_test.go
│       │   ├── writer.go                  (event dispatch, inline/block styling)
│       │   ├── table.go                   (table rendering: live repaint,
│       │   │                               width limiting, wrapping)
│       │   ├── util.go                    (VisibleLen, RenderInline, WrapContent,
│       │   │                               padding helpers)
│       │   ├── tty.go                     (terminal detection, width)
│       │   └── writer_test.go
│
├── examples/
│   ├── basic/
│   └── streaming/
│
├── go.mod
├── go.sum
├── README.md
└── Makefile
```

---

# Implementation Status

## Features Supported

| Section | Feature | Status |
|---------|---------|--------|
| 2.1 | Characters and lines | ✅ |
| 2.2 | Tabs (tab-stop-of-4 for indented code blocks, structural delimiters, HR markers) | ✅ |
| 2.3 | Insecure characters | ✅ |
| 2.4 | Backslash escapes (context-aware — NOT decoded in code spans/blocks) | ✅ |
| 2.5 | Entity and numeric character references (context-aware) | ✅ |
| 3.1 | Precedence — block indicators take priority over inline at `lineStart` | ✅ |
| 3.2 | Container/leaf blocks | ✅ |
| 4.1 | Thematic breaks (`---`, `***`, `___` with up to 3 leading spaces) | ✅ |
| 4.2 | ATX headings (`#` to `######`, with closing `#` strip, inline parsing inside) | ✅ |
| 4.3 | Setext headings (`===`, `---`) | ✅ |
| 4.4 | Indented code blocks (4+ spaces/tab, tab-stop-of-4) | ✅ |
| 4.5 | Fenced code blocks (`` ``` `` and `~~~`) | ✅ |
| 4.6 | HTML blocks (types 1-5: `<pre>`/`<script>`/`<style>`/`<textarea>`, `<!-- -->`, `<? ?>`, `<!DOCTYPE>`, `<![CDATA[`) — detected at line start, emitted raw with dim ANSI styling | 🟡 |
| 4.7 | Link reference definitions & reference links — single-line LRDs detected and rendered as `[Link Def: label → url]` (dimmed); `[text][label]` and `[text][]` reference links rendered with link styling and `[→ label]` hint | 🟡 |
| 5.1 | Block quotes (`>`) | ✅ |
| 5.2 | List items (bullet `-`/`*` + ordered `1.`/`1)`) | ✅ |
| 6.1 | Code spans (multi-backtick) | ✅ |
| 6.2 | Emphasis (`*`/`_` for bold/italic with nested-stacking, left/right flanking checks for both, intraword `_` prevention, multiple-of-3 rules, Rule 17 code/link precedence) | ✅ |
| 6.3 | Inline links `[text](url)` — URL parsing with angle brackets `<...>`, balanced parentheses, space/control-char rejection; title parsing (`"title"`, `'title'`, `(title)`); balanced brackets in link text (depth tracking); `\\`, `\[`, `\]` escapes in link text; leading whitespace before URL; title display in renderer | ✅ |
| 6.7 | Hard line breaks (`\` at end of line) | ✅ |
| GFM | Tables (`| columns |`, alignment, live repaint on TTY) | ✅ |
| GFM | Strikethrough (`~~`) | ✅ |

## Sections Partially Supported

| Section | Feature | Completed | Remaining Gap | Reason |
|---------|---------|-----------|---------------|--------|
| 4.6 | HTML blocks | Types 1-5 (end-tag terminated) — detected at line start, raw content emitted with dim ANSI styling, end detected via in-line end condition | Types 6-7 (blank-line terminated: `<div>`, `<table>`, `<p>`, and 50+ other block-level tags; also complete open/close tags on their own line) and HTML blocks inside blockquotes (`> <div>`) / list items (`- <div>`, Ex 174, 175) | Types 6-7 need blank-line lookahead across chunk boundaries; type 7 requires `paragraphActive` tracking; container nesting is deferred along with 5.3 |
| 4.7 | Link reference definitions / reference links | Single-line LRDs (`[label]: url "title"`) detected at line start, rendered as `[Link Def: label → url]` with dim styling. Full (`[text][label]`) and collapsed (`[text][]`) reference links rendered with link styling + `[→ label]` hint | Multi-line LRDs (URL on next line, title spanning lines) entered via `LinkRefDefState` but URL collection incomplete. Shortcut reference links (`[label]` alone) not supported — cannot distinguish from plain bracketed text without definition lookup | Multi-line LRDs need title-line continuation detection; shortcut links require a definition store (anti-streaming) |

## Sections Deferred

| Section | Feature | Reason |
|---------|---------|--------|
| 4.6 | HTML blocks types 6-7, HTML in containers | See Sections Partially Supported above |
| 4.7 | Link reference definitions (multi-line, shortcut) | See Sections Partially Supported above |
| 5.3 | Lists (nesting, tight/loose) | Requires list-context stack |
| 6.4 | Images | Same complexity as links + alt text |
| 6.5 | Autolinks | Angle-bracket token needed |
| 6.6 | Raw HTML inline | Same reasoning as 4.6 |
| GFM | Task lists | Deferred |

## Events & Payloads

Event payloads use typed fields instead of string encoding:

| Event | Fields used |
|-------|-------------|
| `TextEvent` | `Value` |
| `HeaderStartEvent`, `HeaderEndEvent` | `Level` (1–6) |
| `TableStartEvent` | `Cells` (header), `Widths`, `Aligns` |
| `TableRowEvent` | `Cells` |
| `LinkEvent` | `Value` (text), `URL`, `Title` |
| `HTMLBlockStartEvent`, `HTMLBlockEndEvent` | (no payload) — emitted as raw with dim style |
| `LinkRefDefEvent` | `Value` (label), `URL` (destination) — rendered dimmed as `[Link Def: label → url]` |
| `LinkRefEvent` | `Value` (link text), `URL` (reference label) — rendered with link style + `[→ label]` hint |

There is no `Extra` field and no `\x00` string encoding.

## Error Propagation

- `Writer.Handle(e Event) error` — write errors propagate through the entire pipeline
- `Pipeline.Write/Flush/Close` return the first write error
- `Reset()` emits `\033[0m` for terminal safety

---

# Parser Internals

## State Machine & Module Map

```
                              ┌─────────────────────────────────────┐
     tokens in                │         parser.go: process()        │
     ──────────▶              │                                     │
                              │   switch p.state {                  │
                              │                                     │
              ┌───────────────┤     case NormalState:               │
              │               │       processNormal()               │
              │               │         │                           │
              │               │         ├─ processEscapeOrEntity()  │── escapes.go
              │               │         │                           │   entities.go
              │               │         ├─ processLineStartBlock()  │── blocks.go
              │               │         │                           │
              │               │         ├─ processInlineStart()     │── inlines.go
              │               │         │                           │
              │               │         ├─ processDeferredLineStart │── blocks.go
              │               │         │                           │   tables.go
              │               │         └─ emitTextOrSpecial()     │── parser.go
              │               │                                     │
              │    ┌──────────┤     case HeaderState:               │── blocks.go
              │    │          │       processHeader()               │
              │    │          │                                     │
              │    │          │     case BoldState:                 │── inlines.go
              │    │          │       processBold()                 │
              │    │          │                                     │
              │    │          │     case ItalicState:               │── inlines.go
              │    │          │       processItalic()               │
              │    │          │                                     │
              │    │          │     case StrikethroughState:        │── inlines.go
              │    │          │       processStrikethrough()        │
              │    │          │                                     │
  state        │    │          │     case InlineCodeState:          │── inlines.go
  transitions  │    │  ┌───────┤       processInlineCode()          │
  (enter/      │    │  │       │                                     │
   exit back   │    │  │       │     case CodeBlockState:           │── blocks.go
   to Normal)  │    │  │       │       processCodeBlock()           │
               │    │  │       │                                     │
               │    │  │       │     case IndentedCodeBlockState:   │── blocks.go
               │    │  │       │       processIndentedCodeBlock()   │
               │    │  │       │                                     │
               │    │  │       │     case BlockquoteState:          │── blocks.go
               │    │  │       │       processBlockquote()          │
               │    │  │       │                                     │
               │    │  │       │     case TablePendingState:        │── tables.go
               │    │  │       │       processTablePending()        │
               │    │  │       │                                     │
               │    │  │       │     case TableBodyState:           │── tables.go
               │    │  │       │       processTableBody()           │
               │    │  │       │                                     │
               │    │  │       │     case SetextPendingState:       │── blocks.go
               │    │  │       │       processSetextPending()       │
               │    │  │       │                                     │
               │    │  │       │     case LinkTextState:            │── inlines.go
               │    │  │       │       processLinkText()            │
               │    │  │       │                                     │
                │    │  │       │     case LinkURLState:             │── inlines.go
                │    │  │       │       processLinkURL()             │
                │    │  │       │                                     │
                │    │  │       │     case HTMLBlockState:           │── blocks.go
                │    │  │       │       processHTMLBlock()            │
                │    │  │       │                                     │
                │    │  │       │     case LinkRefDefState:           │── blocks.go
                │    │  │       │       processLinkRefDef()           │
                │    │  │       │                                     │
                │    │  │       │   }  // end switch                 │
               │    │  │       │                                     │
               │ ┌──┘  │       │   if buf unchanged && state        │
               │ │     │       │      unchanged → break (await      │
               │ │     │       │      more tokens)                   │
               │ │     │       └─────────────────────────────────────┘
               │ │     │
               │ │     │  events out
               │ │     │  ──────────▶  Renderer
               │ │     │
               │ │     └── close.go: finalizeState()
               │ │          Flush / CloseStates / safeFlush
               │ │          (unified recovery, closes open spans,
               │ │           drains pending table/setext/link data)
               │ │
               │ └── helpers.go: token buffer ops, indent, HR check,
               │     flanking, structural whitespace
               │
               └── events.go + state.go: Event/State type definitions
```

**Module responsibilities:**

| File | Handles |
|------|---------|
| `parser.go` | Core dispatch loop (`process`), `processNormal` with ordered stages, `emitTextOrSpecial`, buffer pattern matching |
| `blocks.go` | ATX/setext headings, thematic breaks, code blocks (fenced + indented), blockquotes, bullet/ordered lists, HTML blocks, link reference definitions |
| `inlines.go` | Bold, italic, strikethrough, inline code, inline links, reference links |
| `tables.go` | Table detection, cell extraction (`readTableCells`), separator alignment parsing |
| `close.go` | `Flush`, `CloseStates`, `safeFlush`, unified `finalizeState` recovery |
| `helpers.go` | Token buffer helpers, indentation (`equivIndent`), list prefix parsing, HR check, flanking checks |
| `escapes.go` | Backslash escape handling (context-aware — not decoded in code states) |
| `entities.go` | Numeric/named entity references (context-aware, CommonMark-validated) |
| `events.go` | `EventType` enum, `Event` struct |
| `state.go` | `State` enum |

**Flow — Normal state processing (`processNormal`):**

```
┌─────────────────────────────┐
│  1. processEscapeOrEntity() │  BackslashToken / AmpersandToken?
│     escapes.go, entities.go │  → decode, emit as text
└──────────┬──────────────────┘
           │ not handled
┌──────────▼──────────────────┐
│  2. processLineStartBlock() │  p.lineStart only
│     blocks.go               │  HR (all 3 markers), ATX heading,
│                             │  blockquote, dash/star bullet,
│                             │  ordered list, code fence
└──────────┬──────────────────┘
           │ not handled
┌──────────▼──────────────────┐
│  3. processInlineStart()    │  Unguarded (mid-line or line-start)
│     inlines.go              │  Bold (** / __), italic (* / _),
│                             │  strikethrough (~~), inline code (`),
│                             │  inline link ([text]...)
└──────────┬──────────────────┘
           │ not handled
┌──────────▼──────────────────┐
│  4. processDeferredLineSt.  │  p.lineStart only
│     tables.go, blocks.go    │  Table candidate (|), indented code
│                             │  block (4+ spaces/tab), setext
│                             │  candidate
└──────────┬──────────────────┘
           │ not handled
┌──────────▼──────────────────┐
│  5. emitTextOrSpecial()     │  Fallback: emit tokens as plain text
│     parser.go               │  Checks bufferHasPattern at each step
└─────────────────────────────┘
```

Each stage returns `([]Event, bool)` — events plus a `handled` flag. When a stage
handles the token (returns `true`), subsequent stages are skipped. This makes rule
precedence explicit: block-level dispatch runs before inline, line-start rules run
before mid-line rules.

## Parser Struct: Embedded Contexts

The `Parser` struct groups related state into focused embedded types:

```go
type Parser struct {
    state           State
    tokenBuffer                   // buf []Token, eof bool
    lineContext                   // lineStart, prevChar, contentIndent
    blockContext                  // headerLvl, fenceLen, fenceChar, codeBlockFirst
    emphasisContext               // italicOpener, boldOpener, emphStack []emphasisFrame
    tableContext                  // tableHeaderBuf, tableColWidths, tableColAligns
    setextContext                 // setextWaiting, setextBuf
    linkContext                   // linkBuf, linkURLBuf, linkBracketConsumed
}
```

`Reset()` zeroes all structs by assignment, keeping initialization in one place.

## processNormal: Dispatch Stages

`processNormal()` is split into explicit ordered stages, each returning `([]Event, bool)` — events and a `handled` flag:

1. **`processEscapeOrEntity`** — backslash escapes and `&` entities
2. **`processLineStartBlock`** (guarded by `lineStart`) — HR (all 3 markers), ATX headings, blockquotes, dash/stars/ordered-list bullets, ordered lists, code fences
3. **`processInlineStart`** (unguarded) — bold (`**`, `__`), italic (`*`, `_`), strikethrough (`~~`), inline code (`` ` ``), inline links (`[`)
4. **`processDeferredLineStart`** (guarded by `lineStart`) — tables (`|`), indented code blocks (4+ space/tab equiv), setext candidates
5. **`emitTextOrSpecial`** — fallback: emit plain text, checking `bufferHasPattern` at each step

This makes rule order explicit and keeps the dispatch logic testable in isolation.

## Token Buffer Helpers

All `p.buf` access is centralized through methods on `tokenBuffer`:

| Method | Purpose |
|--------|---------|
| `appendTokens([]Token)` | Append tokens to buffer |
| `bufferedLen() int` | Current buffer length |
| `hasBufferedTokens() bool` | `len(buf) > 0` |
| `prependTokens(...Token)` | Prepend tokens (replaces manual slice-alloc patterns) |
| `consume(n int)` | Remove n tokens from buffer head |
| `hasNewline() bool` | Check for newline in buffer |
| `checkConsecutive(TokenType, int) (bool, bool)` | Check n consecutive tokens; returns `(matched, waiting)` |
| `countConsecutive(TokenType) int` | Count consecutive tokens of given type |

`prependTokens` replaces the `p.buf = append([]Token{{...}}, p.buf...)` pattern used in entity/escape handlers and code block info string processing.

## Unified Close/Flush

The three finalization paths share one implementation via `finalizeMode`:

```go
type finalizeMode int

const (
    finalizeClose finalizeMode = iota  // CloseStates: close all open spans
    finalizeFlush                      // Flush: drain pending states, close spans
    finalizeSafe                       // safeFlush: panic recovery — same as close + buffer drain
)
```

- `CloseStates()` → `finalizeState(finalizeClose)`
- `Flush()` delegates `TableBodyState` to `finalizeState(finalizeFlush)` directly; other pending states (`TablePendingState`, `SetextPendingState`, `LinkTextState`, `LinkURLState`) also route through it
- `safeFlush()` → `finalizeState(finalizeSafe)` + buffer-as-text drain + `Reset()`

`finalizeState` is the single recovery path — each state closes/open-flushes once, eliminating the duplicated table/setext/link logic that was previously in both `Flush` and `CloseStates`.

For table cell scanning and rendering details, see `dev_docs/Implementation_Tables.md`.

---

# Robustness

## Streaming Chunk-Boundary Safety

| Mechanism | Purpose |
|-----------|---------|
| **UTF-8 byte splitting** | `Pipeline.Write` scans last 4 bytes for incomplete UTF-8 sequence, buffers the partial bytes |
| **`checkConsecutive` / `waiting` flag** | Multi-token sequences (`**`, `` ``` ``/`~~~`, `---`) return `waiting=true` if chunk ends prematurely — parser suspends, waits for next chunk |
| **`eof` flag** | `Close()` → `Flush()` sets `eof=true`, forcing all pending ambiguous sequences to resolve as literal text |
| **Persistent link buffers** | `linkBuf` and `linkURLBuf` on `Parser` survive chunk boundaries — no data loss when `[text](ur)` is split |
| **Setext lookahead** | `setextWaiting`/`setextBuf` buffer until `\n`; `Flush`/`CloseStates` drain as plain text if no underline follows |

## Tests

- `golden_test.go`: 7 test suites, 31 one-shot golden cases, 10 cross-boundary cases, 8 ANSI pairing checks, 5 reset cleanup checks
- `commonmark_spec_test.go`: 655 CommonMark examples — verifies no crash, text preservation
- `characterization_test.go`: streaming boundary tests for parser close/flush behavior (bold, italic, inline code, fenced code, table, setext, link)

---

# Bug Fixes

| Category | Bug | Fix |
|----------|-----|-----|
| **Emphasis (§6.2)** | Nested emphasis not supported — `*foo **bar** baz*` rendered as fragmented italic spans instead of italic containing bold; `***foo***` rendered as bold with literal asterisks; `~~**foo**~~` not rendered as combined strikethrough+bold | Implemented 3-frame emphasis stack (`emphasisFrame` with state/closerType/closerLen) within `NormalState`. Stack tracks pending closers; `tryEmphasisCloser` pops matching frames at delimiter boundaries; `tryEmphasisStar`/`tryEmphasisUnderscore`/`tryEmphasisTilde` handle openers, closers, and nested openers. Renderer tracks `inBold`/`inItalic`/`inStrikethrough` flags and emits composite ANSI SGR codes (`\033[1;3m`, `\033[1;9m`, `\033[1;3;9m`). Closer matching uses `checkConsecutive` with chunk-boundary awareness — returns `handled=true` (waiting) only when buffer might receive more tokens of the same type. `tryBulletOrBold` updated to handle 3+ star openers (push Italic+Bold frames). — Files: `emphasis.go` (new), `parser.go`, `blocks.go`, `recognizers.go`, `inlines.go`, `close.go`, `render/writer.go` |
| **Emphasis (§6.2)** | `*` at line start not detected as emphasis opener — `*(**foo**)*` rendered as literal `*` instead of italic; `*_foo_*` and `_*foo*_` showed literal outer delimiters (§6.2 Ex 393, 461, 463) | `tryBulletOrBold` fallback removed: instead of consuming single `*` as literal text, returns `nil` to let `processInlineStart` → `tryEmphasisStar` detect emphasis. Removed `!p.lineStart` guard on StarToken in `processInlineStart` so that `*` at line start reaches emphasis detector after block-level dispatch. Allowed cross-character emphasis nesting (`*` containing `_` and vice versa) by checking top frame's `closerType` instead of blocking all `ItalicState`/`BoldState` duplicates — same-character nesting still blocked (requires CommonMark rules 13-16). |
| **Emphasis (§6.2)** | `*foo**` rendered as literal text instead of `<em>foo</em>*`; `*foo**bar*` lost literal `**` inside italic; `**foo \"*bar*\" foo**` mis-nested emphasis/strong spans (§6.2 Ex 443, 412, 395) | `tryEmphasisCloser` italic-closer disambiguation for `**` boundaries: when top frame is Italic(StarToken,1) and count ≥ 2 with no Bold on stack, skip close if the `**` could be a valid Bold opener (`hasMatchingCloser` for 2 stars after) or if there's a later single-`*` closer (`hasMatchingCloser` for 1 star after the run). Otherwise consume 1 `*` as Italic closer, leaving the remaining `*` as literal. Moved `tryEmphasisCloser` after `***` check in `tryEmphasisStar` so 3-star combined openers take priority. |
| **Emphasis (§6.2)** | Flanking algorithm oversimplified — `isLeftFlanking` only checked "next char not space/tab" for `_` only, `isRightFlanking` entirely absent, `*` never flanking-checked. Caused false emphasis on `a*"foo"*`, `*foo bar *`, `** foo bar**`, etc. (Ex 351, 366, 379, 380, 391, 421 and many more). | Implemented full `isLeftFlankingRun` and `isRightFlankingRun` with ASCII character classification (`isWhitespaceByte` including NBSP `\xa0`, `isPunctByte`). Both checks integrated into `tryEmphasisStar` (all `*`/`**`/`***` opener paths), `tryEmphasisUnderscore` (all `_`/`__`/`___` opener paths), and `tryEmphasisCloser` (all closer paths). Delimiter character itself excluded from "preceding punctuation" in `isRightFlankingRun`. — Files: `helpers.go`, `emphasis.go` |
| **Emphasis (§6.2)** | `prevChar` not updated by `handleBackslash`, entity handlers, `processInlineCode`, `processLinkURL`, `processLinkRefLabel`, or `flushLinkAsText` — caused stale flanking context after escapes/entities/code-spans/links, breaking closers immediately after these constructs (Ex 437, 440, 441 regression). | Added `prevChar` updates to all escape/entity/code/link text emission paths. Added `markerDelimiterByte` helper for emphasis delimiter character. — Files: `escapes.go`, `entities.go`, `inlines.go` |
| **Emphasis (§6.2)** | `prevChar` also not updated by emphasis openers/closers — after consuming delimiter tokens, flanking checks for subsequent delimiters saw stale preceding character. | Added `prevChar` updates to all opener and closer delimiter consumption paths in `tryEmphasisStar`, `tryEmphasisUnderscore`, `tryEmphasisTilde`, and `tryEmphasisCloser`. Later reverted opener `prevChar` (kept only closer + literal text `prevChar`) to prevent same-run closing (Ex 421). — Files: `emphasis.go`, `blocks.go` |
| **Emphasis (§6.2)** | `tryBulletOrBold` missing flanking checks for all emphasis opener paths — `**` bold opener, `***` combined opener, and single-`*` italic opener all bypassed left-flanking validation at line start (Ex 379). | Added `isLeftFlankingRun` to all `tryBulletOrBold` emphasis paths. — File: `blocks.go` |
| **Emphasis (§6.2)** | Rule 17 violated — `processInlineStart` checked `*`/`_` before `` ` ``/`[`, so code spans and links didn't group tighter than emphasis (Ex 404, 419, 422, 433, 473, 474). | Reordered `processInlineStart` dispatch: `BacktickToken` and `LeftBracketToken` checked BEFORE `StarToken` and `UnderscoreToken`. — File: `parser.go` |
| **Emphasis (§6.2)** | Intraword underscores not prevented — `foo_bar_` and `foo__bar__` rendered as italic/bold instead of literal underscores (Ex 374, 375). Rules 2/4/6/8 (can open/close only if not-right-flanking or preceded/followed by punctuation) not implemented. | Added `canUnderscoreOpen(runLen)` and `canUnderscoreClose(runLen)` — combine left/right-flanking with Rules 2/4 conditions. Replaced `isLeftFlankingRun(UnderscoreToken, count)` with `canUnderscoreOpen(count)` throughout `tryEmphasisUnderscore`. UnderscoreToken closer path in `tryEmphasisCloser` uses `canUnderscoreClose` instead of plain `isRightFlankingRun`. — File: `emphasis.go` |
| **Emphasis (§6.2)** | Optimistic openers without valid closer — `*`/`_`/`**`/`__` opened emphasis even when no closer existed that could actually close (e.g., closer preceded by whitespace for stars, intraword for underscores). Caused false emphasis on `*foo bar *`, `_foo_bar`, `**foo bar **`. | Added `hasFlankingCloser` — scans ahead for a closer that (a) is from a separate delimiter run (had intervening content), and (b) is not preceded by whitespace (for stars) or not intraword (for underscores). Replaced `hasMatchingCloser` with `hasFlankingCloser` in all emphasis opener paths. When `hasFlankingCloser` fails but `hasMatchingCloser` succeeds, the closer exists but is invalid → emit delimiters as literal. — Files: `escapes.go`, `emphasis.go`, `blocks.go` |
| **Emphasis (§6.2)** | Multiple-of-3 rule (Rules 9/10) not implemented — `**foo*` and `__foo_` should produce `*<em>foo</em>` and `_<em>foo</em>` but incorrectly rendered as literal or bold (Ex 409, 442). | Added `multipleOf3RuleBlocks(openerLen, closerLen) bool` — sum is multiple of 3 and not both multiples → blocked. Applied in `**`/`__` opener paths: when no 2-char closer but 1-char closer exists AND mult-of-3 blocks, fallback to italic with literal `*`/`_` (emit TextEvent for the literal character + ItalicStartEvent, consuming 2 at once for correct event ordering). Also applied in `tryBulletOrBold`. — Files: `emphasis.go`, `blocks.go` |
| **Emphasis (§6.2)** | Sub-parser boundary issues — `parseInlineLine` sub-parsers (used for setext flush, header inline content, table cells) didn't set `eof=true`, so flanking checks at buffer end returned permissive results, causing false emphasis openings. Also affected single-`*` italic openers when setext candidate swallowed the line (Ex 380, 437 regression). | Set `eof = true` in `parseInlineLine` sub-parsers before `Parse()`. — File: `blocks.go` |
| **Thematic breaks** | `**\n` / `__\n` treated as bold openers (§4.1 Ex46) | `hasMatchingCloser` + `hasNewlineIn` guard before entering BoldState |
| | Leading spaces (` ***`) blocked HR detection (§4.1 Ex47,52) | Blanket HR check at `lineStart` before per-marker dispatch |
| | `_ _ _ _ a` — `_` + space started italic (§4.1 Ex55) | `isLeftFlanking()` check: `_` followed by whitespace is not left-flanking |
| | HR not detected inside list items (§4.1 Ex61) | HR check in `!lineStart` StarToken/UnderscoreToken paths |
| **ATX headings** | `####### foo` dropped `foo` (§4.2 Ex63) | Pre-check consumes 7+ hashes as literal text |
| | ` ### foo` ignored leading spaces (§4.2 Ex68,71) | `leadingSpaceCount` + hash-detection after 1–3 spaces |
| | No inline parsing inside headings (§4.2 Ex66,76) | `parseInlineLine()` on heading body for emphasis/escapes/code/entities |
| | `## foo ##` kept trailing hashes (§4.2 Ex71–74,79) | `stripClosingHashSequence()` — unescaped hashes preceded by space |
| | `#\n` not recognized as empty heading (§4.2 Ex79) | `NewlineToken` accepted as valid delimiter |
| **Backslash escapes** | Escapes processed in tokenizer → stripped inside code spans/blocks (§2.4) | Moved to parser (`escapes.go`); context-gated: not decoded in code states |
| **Entities** | `&amp;` decoded everywhere (§2.5) | `AmpersandToken` with context-gated decoding; CommonMark-validated |
| **Tabs** | Tabs not treated as equivalent spaces for block structure (§2.2) | `equivIndent` with tab-stop-of-4; `hasStructuralWhitespace` accepts `TabToken` |
| **Emphasis** | `_italic_` and `__bold__` mid-line not rendered | `bufferHasPattern` includes `UnderscoreToken` |
| | Italic/bold state didn't track opener type | `italicOpener`/`boldOpener` fields track `*` vs `_` for correct closing |
| | Italic closed at line break, breaking emphasis across lines (§4.3 Ex81,82) | `processItalic` now spans soft line breaks (matches `processBold`); `hasMatchingCloser` gained `spanNewlines` flag (stops at blank lines) |
| **Setext headings** | `=` underline required length ≥ 3 (§4.3 Ex83) | `checkSetextUnderline` accepts any `=`/`-` sequence (length ≥ 1) |
| | Underline leading spaces (`  ===`) rejected (§4.3 Ex84) | Skip up to 3 leading spaces on the underline line |
| | `checkSetextUnderline` consumed tokens on failure, losing `---` → stray `-` (§4.3 Ex88) | Non-consuming scan; consume only on success |
| | Multi-line setext content treated as paragraph + orphan heading (§4.3 Ex81,82,95) | `processSetextPending` appends continuation text lines to `setextBuf` (joined with newline); strips leading spaces (first line) and trailing whitespace (last line) |
| | Backslash at EOL of setext content dropped (hard-break eaten by tokenizer) (§4.3 Ex90) | Tokenizer emits `BackslashToken`+`NewlineToken`; `handleBackslash` detects hard break in paragraph context; setext collector keeps trailing `\` literal |
| | HR rendered inside a closed blockquote (prefix leak) (§4.3 Ex92) | `processBlockquote` emits `BlockquoteEndEvent` before the trailing `NewlineEvent` |
| **Blockquote** | Lazy continuation lines not supported (§4.3 Ex93) | *Deferred* — streaming pipeline feeds input line-by-line, so the next line is unavailable when the blockquote newline is processed; see "Known Limitations" |
| **Indented code blocks** | Blank lines (whitespace-only) between chunks exited the code block prematurely (§4.4 Ex111) | `isBlankLineTokens` check in `processIndentedCodeBlock`: blank lines are emitted as code block content, not treated as block-ending lines |
| | 4-space-indented list markers (`    - bar`, `    1. foo`) entered code block instead of being treated as list items (§4.4 Ex109) | `peekEquivIndent` (non-destructive) + `isListStartAfterIndent` in `processDeferredLineStart`: list item interpretation takes precedence per spec rule; `handleIndentedList` / `handleIndentedListRemaining` dispatch to the appropriate bullet/ordered-list handler |
| **Indented code blocks** | List paragraph after blank line (`- foo\n\n    bar\n`) treated as code block instead of list paragraph (§4.4 Ex108) | *Deferred* — requires list nesting / continuation context (tracking whether preceding line was a list item) |
| **Fenced code blocks** | Indented fences (` ``` `/`  ``` `/`   ``` `) not detected; lines with leading spaces fell through to `SetextPendingState` (§4.5 Ex131–133) | `tryFencedCodeBlock()` in `processLineStartBlock`: checks for 0–3 spaces/tabs before fence chars, properly counts spaces (not tokens) for indentation tracking |
| | `codeBlockIndent` field for content line indentation stripping: up to N spaces removed from each content line when opening fence has N spaces of indent (§4.5 Ex131–133) | `stripLineIndent()` helper; content lines pass through indentation stripping path in `processCodeBlock` |
| | Backticks in info string not detected — invalid fences like `` ``` ``` `` treated as code blocks instead of inline code (§4.5 Ex138,145) | `tryFencedCodeBlock()` and `processBacktickStart` scan remaining line tokens for `BacktickToken`; if found, fence is rejected and falls through to inline code handling |
| | Closing fence with 4+ spaces of indentation (`    ``` `) incorrectly closed the code block because whitespace tokens didn't set `p.lineStart = false` (§4.5 Ex137) | `processCodeBlock` pure-whitespace handler now sets `p.lineStart = false`; `tryCloseCodeFence()` validates indentation ≤ 3 spaces and checks only whitespace follows fence chars |
| | Closing fence without trailing newline or with non-whitespace content (e.g. `` ``` aaa ``) falsely matched as a closing fence (§4.5 Ex147) | `tryCloseCodeFence()` validates that only whitespace/tabs follow the fence chars before newline; non-matching lines fall through to content handling |
| | Info string containing `#` (HashToken) caused partial consumption — the `#` fell through to code content instead of being appended to the info string (§4.5 Ex143) | `HashToken` now handled in the `codeBlockFirst` info string branch, writing `"#"` to `infoBuf` |
| **Inline code** | `TrimPrefix`/`TrimSuffix` in `processInlineCode` modified a local copy of the event instead of the slice element — inline code content spaces were not trimmed | Changed to index-based assignment (`events[0].Value = ...`) so modifications apply to the slice |
| | Inline code content consisting entirely of spaces (e.g. `` ``` ``` `` → `<code> </code>`) was incorrectly trimmed to empty | Added `contentOnlySpaces` check: spaces are preserved when the entire inline code body is space characters |
| **HTML blocks** | HTML tags split by tokenizer at `>`, `-`, `[`, `]` — start patterns (`<!--`, `<pre>`, `<?`, etc.) undetectable from first TextToken alone (§4.6) | `tryHTMLBlock` collects full first-line tokens up to newline, rebuilds text via `rebuildLineText`, checks all type 1-5 patterns against the reassembled line |
| | First-line content lost because `tryHTMLBlock` consumed the initial TextToken but `processHTMLBlock` only processed remaining tokens | `tryHTMLBlock` now consumes all tokens on the first line (including newline), emits the entire line as text events together with `HTMLBlockStartEvent`, and checks end condition on the same line |
| | Types 6-7 and HTML blocks inside containers not detected | *Deferred* — requires `paragraphActive` tracking (type 7) and blank-line lookahead across chunk boundaries (both types); container nesting deferred along with 5.3 |
| **Link reference definitions / reference links** | `[label]: url "title"` lines not detected; `[text][label]` and `[text][]` reference links emitted as raw text (§4.7, §6.3 reference links) | Single-line LRD detection via `tryLinkRefDef` + `parseLinkRefDefLine` at line start; reference link detection via `processLinkRefLabel` in `processLinkText` after `]` followed by `[` |
| | Escaped `\]` in label not recognized (`[Foo\]]:url` treated as label ending at first `]`) | `findUnescapedBracket` skips `\]` sequences when searching for closing bracket |
| | Multi-line LRDs (URL on next line) and shortcut `[label]` reference links not supported | *Deferred* — multi-line needs title continuation tracking; shortcut links require definition store (anti-streaming) |
| **Paragraphs** | Continuation line leading whitespace (spaces/tabs) preserved literally instead of being stripped (§4.8 Ex222,223) | `stripLeadingWhitespaceTokens()` applied to continuation line tokens in `processSetextPending` before appending to `setextBuf` |
| **Block quotes** | Lazy continuation lines (non-`>` paragraph continuation text) closed the blockquote prematurely (§5.1 Ex232,233,238) | `processBlockquote` reworked: re-entry path distinguishes > markers, block terminators, and lazy continuation; added `isBlockquoteTerminator` and `consumeBlockquoteMarker` helpers |
| | Empty blockquote line (`>\n`) followed by non-`>` line did not close the blockquote (§5.1 Ex249) | `blockquoteHadBlank` flag in `blockContext`; empty `>\n` sets it, next non-`>` line closes the blockquote |
| | Indented `    - bar` after blockquote paragraph incorrectly terminated the blockquote (§5.1 Ex238) | `isBlockquoteTerminator`: 4+ spaces with list-marker-like content treats as paragraph continuation (indented code blocks cannot interrupt paragraphs); pure-whitespace tokens never terminate |
| **List items** | `-\n` / `*\n` at line start neither a list item nor handled — emitted as raw text and caused subsequent lines to trigger false setext heading (§5.2 Ex278) | `processSetextPending`: list-marker precedence check before setext underline detection — if the candidate line starts with `-` or `*` followed by newline or structural whitespace, the pending setext is flushed as paragraph text, avoiding the false setext heading |
| | Indented code blocks after a list marker not detected on the first line (`1.     indented code` → code block starts at W+1 spaces beyond marker) (§5.2 Ex273,274) | `tryBullet`, `tryBulletOrBold`, and the ordered-list `TextToken` handler in `processLineStartBlock`: after consuming the list marker and mandatory space, check for ≥4 spaces in the remaining content — if found, enter `IndentedCodeBlockState` and emit `CodeBlockStartEvent` with the code content |
| | Single dash at line start followed by text (non-bullet fallthrough) left `p.lineStart` true, causing subsequent dash to be re-evaluated as a list item (§5.2 Ex281) | `tryBullet` fallthrough path now sets `p.lineStart = false` (matching `tryBulletOrBold` behavior) |
| **Lists** | Lines with 4+ spaces of indentation followed by a list marker (e.g. `    - e`, `    3. c`) were incorrectly treated as new indented list items (§5.3 Ex312,313) | Per §5.3 spec rule "list items must not be preceded by more than three spaces of indentation": when `peekEquivIndent` detects 4+ spaces before a list marker, enter `IndentedCodeBlockState` instead of `handleIndentedList` — the 4+ spaces make it either an indented code block (with preceding blank line) or paragraph continuation text |
| **Code spans** | Closing backtick string not validated — matched any n consecutive backticks regardless of surrounding backtick context (§6.1 Ex330,331,340) | `processInlineCode`: before accepting a closing backtick string, validate that it is neither preceded nor followed by a backtick token (`prevWasBacktick` flag + lookahead check) |
| | Newlines inside code spans closed the span instead of being converted to spaces per spec (§6.1 Ex336) | `processInlineCode` newline handler: emit TextEvent(" ") instead of closing the code span |
| | Leading/trailing space stripping applied independently to start and end — spec requires BOTH ends to have spaces (§6.1 Ex332) | Space stripping now only applies when both the first and last events have leading/trailing spaces respectively |
| | Multi-backtick opener (` ``` `) entered `InlineCodeState` without checking if a matching closer exists (§6.1 Ex347) | *Deferred* — requires lookahead across chunk boundaries; CloseStates can only append events, can't prepend opening backticks. For n=1 the existing `hasMatchingCloser` check handles the single-chunk case |
| **Links (§6.3)** | Balanced brackets in link text — `[link [foo [bar]]](/uri)` closed at first `]`, producing wrong link text (Ex 512) | Added `linkDepth int` to `linkContext`. Rewrote `processLinkText` to track `[`/`]` nesting depth instead of matching first `]`. When `]` at depth > 0: decrement depth, continue collecting. When `]` at depth == 0: real closing bracket → check for `(` or `[`. Also handles `\[` and `\]` escaped brackets and `\\` (escaped backslash) in link text. — Files: `parser.go`, `inlines.go` |
| **Links (§6.3)** | URL parsing: no angle bracket handling — `<...>` brackets left in URL; no balanced parentheses tracking — `foo(and(bar))` stopped at first `)`; no space-in-URL detection — `/my uri` rendered as link; no title parsing — `/url "title"` merged title into URL (Ex 482–510) | Rewrote `processLinkURL` with phased approach: (1) collect URL in angle-bracket or naked mode, (2) whitespace → transition to title-or-close phase, (3) title parsing, (4) closing `)`. Angle bracket mode: strips `<` and `>`, validates no newlines, handles escaped `\>`. Naked URL mode: tracks balanced parentheses via `urlParenDepth`, detects spaces/control chars and flushes as text. Title parsing: `"title"`, `'title'`, `(title)` delimiters with backslash escape and entity resolution. Added `urlAngleBracket`, `urlParenDepth`, `urlDone`, `urlHadNewline`, `linkTitleBuf` to `linkContext`. Extracted `emitLinkEvent()` and `collectTitle()` helpers. Added `skipURLWhitespace()` for whitespace-after-URL handling. — Files: `parser.go`, `inlines.go`, `events.go` |
| **Links (§6.3)** | Leading whitespace before URL — `[link](   /uri)` falsely detected as space-in-URL and flushed as text (Ex 510) | Added `spaceIdx == 0 && len(p.linkURLBuf) == 0` guard in the space-detection path: when whitespace appears at the start of a TextToken and no URL has been collected, skip leading spaces and continue URL collection with the remainder. Allows spec-legal leading whitespace between `(` and the destination. — File: `inlines.go` |
| **Links (§6.3)** | Title not rendered in output — parsed correctly by parser but ignored by renderer | Added `Title string` to `Event` struct. Updated `render/writer.go` LinkEvent handler to display ` "title"` after the URL when `e.Title != ""`. — Files: `events.go`, `render/writer.go` |

---

# Refactoring Log

## 2026-06-26 — Dispatch Refactoring: Recognizer Extraction

**Goal**: Reduce coupling in the dispatch layer by extracting construct-recognition into independent, named functions with unified signatures.

**What changed:**

- `processLineStartBlock`: 100 → 40 lines. Each construct check now delegates to a named recognizer function (`tryThematicBreak`, `tryATXHeading`, `tryOrderedList`, etc.). Rule precedence is explicit in the call order.

- `processInlineStart`: 90 → 25 lines. Delegates to `tryStarEmphasis` and `tryUnderscoreEmphasis` for bold/italic detection, keeping direct calls only for type-dispatch (`BacktickToken` → `processBacktickStart`, etc.).

- `processDeferredLineStart`: 30 → 15 lines. Delegates to `tryIndentedCodeOrList` and `trySetextCandidate`.

- **New file `recognizers.go`** (~293 lines):
  - `Recognizer` type: `func(p *Parser) (events []Event, handled bool)`
  - 12 extracted recognizer functions:
    - Block: `tryThematicBreak`, `tryATXHeading`, `tryOrderedList`, `tryIndentedCodeOrList`, `trySetextCandidate`
    - Inline: `tryStarEmphasis`, `tryUnderscoreEmphasis`
    - Adapters: `tryBlockquoteR`, `tryBulletDash`, `tryBulletStarOrOrdered`, `tryTableR`
    - Misc: `tryBacktickInline`, `tryTildeInline`, `tryEscapeOrEntity`
  - `enterState(state)` / `exitToNormal()` — explicit state transition helpers
  - `bufferHasPattern()` — rewritten with structured token-type dispatch

- Net: -216 lines in parser.go, +293 lines in recognizers.go (+77 total).

- All tests pass: CommonMark spec (655 examples), golden tests (31 cases), streaming consistency, cross-boundary, robustness.

**Design intent**: Each recognizer function answers exactly one question ("is this an ATX heading?") and returns events only on match. This makes rule precedence visible, construct detection testable in isolation, and the dispatch flow self-documenting. The `Recognizer` type is a soft convention — no registry, no runtime dispatch table. The explicit call order in `processNormal` and its delegates IS the precedence table.

## 2026-06-27 — §6.2 Emphasis Stack & Refinements

Implemented 3-frame emphasis stack, proper left/right flanking (Rules 1–8), intraword underscore prevention (Rules 2/4/6/8), multiple-of-3 delimiter rule (Rules 9/10), Rule 17 dispatch precedence, and prevChar/flanking fixes across escape/entity/code/link paths. AI test results: 53→67 pass (avg 1.5→1.7/3). See `dev_docs/Implementation_Emphasis_and_Strong_Emphasis.md` for full design and implementation details.

---

# Remaining Work

Can be added without architecture changes:

- Syntax highlighting in code blocks
- Nested lists / tight/loose list distinction (CommonMark 5.3)
- Emphasis: full Unicode character classification (currently ASCII-only; NBSP added), Rules 11/12 (literal `*`/`_` at emphasis boundaries), Rules 15/16 (overlapping spans, same-closer precedence), cross-line emphasis
- TUI mode
- Alternate renderers (HTML, plain text)

The core design — **streaming event pipeline with no AST and small state machines** — fits AI output rendering better than traditional static-document Markdown parsers.

---

# Known Limitations (Setext Headings, §4.3)

| Example | Gap | Reason |
|---------|-----|--------|
| Ex 91, 102 | A setext heading whose content line starts with a non-`TextToken` (e.g. `` ` `` or `\>`) is not buffered into `SetextPendingState`, so the following `---`/`===` line is interpreted as a thematic break rather than a setext underline. The spec rule "setext underline takes precedence over thematic break after a paragraph" requires look-back context the line-oriented dispatcher does not carry. These examples still render acceptably (text + HR). |
| Ex 93 | Blockquote lazy continuation (`> foo\nbar\n===` → blockquote paragraph `foo\nbar\n===`) is not supported. The streaming pipeline splits input on `\n` and feeds each line as a separate `Parse()` call, so when the blockquote newline is processed the continuation line is not yet in the buffer. Implementing this would require a cross-chunk "blockquote pending" state similar to `SetextPendingState`. Listed under deferred work (5.3 lazy continuation). |
| Ex 81, 82 | Emphasis now spans soft line breaks within a heading/paragraph, but the full CommonMark emphasis algorithm (6.2) is still deferred, so flanking/precedence edge cases across blank lines or block boundaries may not match the spec exactly. |

---

# Known Limitations (Blank Lines, §4.9)

| Example | Gap | Reason |
|---------|-----|--------|
| Ex 227 | Blank lines at document start, between blocks, and at document end are emitted as visual newlines rather than being suppressed. The spec says blank lines are "ignored" for block structure, and mdflow's parser correctly ignores them — but the renderer emits them as `\n`, producing extra vertical spacing in terminal output. | Suppressing blank lines in a streaming pipeline would require either a "document start" flag to suppress leading newlines, or renderer-level deduplication of consecutive blank lines. The current behavior preserves content and structure; the extra spacing is cosmetic and does not affect comprehension. Deferred as low-priority visual polish. |

---

# Known Limitations (Block Quotes, §5.1)

| Example | Gap | Reason |
|---------|-----|--------|
| Ex 228–232 | Headings, code blocks, and other block constructs inside blockquotes are rendered as plain text rather than being detected and styled. `> # Foo` emits `# Foo` as blockquote text instead of recognising the ATX heading. | `processBlockquote` emits all content as `TextEvent` without re-entering the main block dispatch. Nesting block constructs inside containers would require recursive parsing or a block-context stack. Deferred as a significant architectural change. |
| Ex 237 | Fenced code blocks inside blockquotes (`> \`\`\`\nfoo\n\`\`\``) are not detected — the opening fence, content, and closing fence are all rendered as blockquote text. | Same reason as above — `processBlockquote` does not re-enter block-level dispatch. |
| Ex 250–251 | Nested block quotes (`> > > foo`) are rendered as a single-level blockquote with literal `>` characters in the text. The parser only supports one level of `>` prefix. | Multi-level blockquote nesting requires a blockquote-depth stack and `>`-counting in `tryBlockquote`. Deferred as a moderate architectural change. |
| Ex 230 | Blockquote markers preceded by up-to-3 spaces (`   > foo`) are not detected because `tryBlockquote` only matches `GreaterToken` at line start, not `TextToken` with leading spaces. The `> ` appears as literal text instead of being converted to a blockquote prefix. | Would require `tryBlockquote` (or `processLineStartBlock`) to inspect leading spaces in `TextToken` before checking for `>`, similar to the ATX heading space-before-hash detection. Minor fix deferred. |

---

# Known Limitations (List Items, §5.2)

| Example | Gap | Reason |
|---------|-----|--------|
| Ex 294–295 | Nested sublists (`- foo\n  - bar\n    - baz`) are not detected — all items render as a flat single-level list. The parser does not track list nesting depth. | Requires a list-context stack to track indentation levels relative to the current list marker. Deferred along with 5.3 (Lists: nesting, tight/loose). |
| Ex 298–299 | List items whose first block is a sublist (`- - foo`, `1. - 2. foo`) are rendered flat — the nested markers appear as literal text. | Requires re-entering block-level dispatch for list item content to detect the inner list marker. Same as above. |
| Ex 260 | Block constructs (lists) inside nested blockquotes (`>>- one`) are not detected — the `>` markers and list marker appear as literal text. | Multi-level blockquote nesting + container nesting are both deferred. |
| Ex 278 | Fenced code blocks inside list items (`- \n  \`\`\`\n  bar\n  \`\`\`\n`) are not detected — the backticks are emitted as inline content. | `processBlockquote` / list item content handling does not re-enter block-level dispatch for code fences. Same architectural limitation as block constructs in blockquotes (Ex 228–232). |
| Ex 292–293 | Blockquote inside list inside blockquote (`> 1. > Blockquote`) has the inner `>` rendered as literal text rather than a nested blockquote. | Requires recursive block-level dispatch in both blockquote and list contexts. Deferred. |
| Ex 300 | ATX heading inside list item (`- # Foo`) — the `#` is rendered as literal text within the list item rather than as a heading. | Block constructs inside list items are not detected; same limitation as fenced code blocks and blockquotes in containers. |

---

# Known Limitations (Lists, §5.3)

| Example | Gap | Reason |
|---------|-----|--------|
| Ex 302 | Changing ordered list delimiter starts a new list (`1. foo\n2. bar\n3) baz` → two `<ol>` blocks). The rendered shows a single continuous numbered list. | The parser does not track the current list's delimiter type to detect delimiter changes. Requires list-context tracking. |
| Ex 306–311 | Tight vs loose list distinction (blank lines between items → paragraphs wrapped in `<p>`). The renderer does not visually distinguish tight from loose lists. | Tight/loose requires tracking blank lines between list items and adjusting paragraph rendering per list item. Deferred with list-context stack. |
| Ex 307, 319, 323, 325, 326 | Nested sublists — inner list items are flattened to the same level as outer items. The parser does not track list nesting depth or indent levels. | Requires a list-context stack with indentation-depth tracking. Deferred as significant architectural change (alongside 5.2 nesting). |
| Ex 316–317 | Two-block list items (paragraph continuation + blank line between blocks within same item) — the extra block is rendered as independent text rather than nested under the same list item. | Requires tracking list item content boundaries and block-level continuation within items. Deferred. |
| Ex 318, 320–321, 324 | Block constructs (fenced code, blockquotes) inside list items — the inner blocks are emitted as inline text or with ANSI artifacts rather than as properly styled blocks. | `processBlockquote` / list item content handling does not re-enter block-level dispatch for nested constructs. Same architectural limitation as block constructs in blockquotes. |
| Ex 312 | Paragraph continuation text indented 4+ spaces (`    - e` as continuation of `d`) rendered as code-styled text rather than plain paragraph continuation. | Without list-context tracking, 4+-space-indented content cannot be distinguished as paragraph continuation vs indented code block. Treated as indented code block as the safer fallback. |

---

# Known Limitations (Emphasis and Strong Emphasis, §6.2)

| Example | Gap | Reason |
|---------|-----|--------|
| Ex 353 | Non-breaking space (NBSP `\xa0`) now correctly treated as whitespace for flanking — added to `isWhitespaceByte`. Other Unicode whitespace characters not yet classified. | ASCII-only character classification with NBSP extension. Full Unicode whitespace/punctuation support is deferred polish. |
| Ex 366, 391 | Delimiters preceded/followed by whitespace correctly prevented from opening/closing emphasis via flanking checks. Optimistic openers validated by `hasFlankingCloser` — opener not opened if closer is invalid. | Resolved via flanking functions + `hasFlankingCloser`. |
| Ex 374, 375 | Intraword underscores correctly prevented — `_` and `__` between letters now emitted as literal text via `canUnderscoreOpen`/`canUnderscoreClose` (Rules 2/4/6/8). Unicode letters (Cyrillic etc.) not yet recognized as "letter" in ASCII-only `isPunctByte`. | ASCII letter detection implemented; full Unicode in `canUnderscoreOpen`/`canUnderscoreClose` is deferred polish. |
| Ex 379, 380 | `**` followed by space at line start (Ex 379) and `**` preceding `"` (Ex 380) correctly emit literal text — left-flanking checks now in `tryBulletOrBold` and sub-parser eof prevents permissive flanking. | Resolved. |
| Ex 394, 404, 419, 422, 433, 473–477, 480–481 | Emphasis containing links/code spans: Rule 17 dispatch reorder moved `BacktickToken`/`LeftBracketToken` before `StarToken`/`UnderscoreToken`. Code spans and links now group tighter than emphasis. Ex 422, 478, 479 now pass. Ex 404, 419, 433 still limited — opener emitted before `[` is seen (streaming); no lookahead to prevent optimistic opening. | Rule 17 partially implemented via dispatch reorder. Full prevention of optimistic emphasis opening before links requires lookahead — streaming-limitation. |
| Ex 420–421, 434–435 | Empty emphasis (`**`, `****`): Ex 421 (`****text`) now correctly emits literal — `hasFlankingCloser` detects same-run closer and prevents opening. Cross-chunk `**` (TestStarStarSplit) may emit literal instead of BoldStart when no closer visible — streaming tradeoff. | Resolved for known-buffer cases; cross-chunk bold may be conservative. |
| Ex 409, 442 | Same-character nesting: Ex 442 (`**foo*` → `*<em>foo</em>`) now correctly handled via `multipleOf3RuleBlocks` + `hasFlankingCloser`. Ex 409 passes via existing heuristics. | Resolved. |
| Ex 405, 423 | Emphasis spanning soft line breaks — streaming pipeline feeds one line per `Parse()` call; closer on next line not in buffer. | Pre-existing streaming limitation. |
| — | Cross-chunk boundary italic — `*` opener at chunk boundary without closer in buffer is emitted as literal text. For Bold, `waiting=true` suspends the parser, but Italic (single `*`) has no waiting mechanism. | Pre-existing streaming limitation. |

# Known Limitations (Code Spans, §6.1)

| Example | Gap | Reason |
|---------|-----|--------|
| Ex 342 | Code span inside link text (`[not a \`link](/foo\`)`) — the code span should take precedence over the link, but the link is detected first because `[` appears before `` ` `` in the token stream. | `processInlineStart` dispatches on the first token; the `[` triggers `LinkTextState` before the backtick is reached. Requires lookahead to detect backtick strings before entering link state. |
| Ex 344, 346 | Code span vs HTML tag / autolink precedence — the backtick takes precedence when it appears first but our dispatch order doesn't correctly resolve these edge cases. | HTML blocks and autolinks are already partially supported; the fine-grained precedence with code spans requires cross-construct lookahead. Deferred. |
| Ex 347 | Multi-backtick code span opener (`\`\`\`foo\`\``) where the matching closer doesn't exist — the opening backticks should be emitted as literal text, but the parser enters `InlineCodeState` optimistically and wraps the content in code styling. | For n≥2, the parser can't distinguish "closer not yet arrived" (streaming) from "closer will never arrive" (unclosed). Re-emitting opening backticks as literal at `CloseStates` would require prepending to already-emitted events, which the streaming architecture doesn't support. |
| Ex 349 | Mixed-length backtick strings (`\`foo\`\`bar\`\``) — the parser opens a code span with the first single backtick and consumes everything as content, while the spec would use the double-backtick pair for a code span containing only "bar". | The parser uses left-to-right matching with the first valid opener and doesn't re-evaluate shorter openers when longer ones are available. Requires a backtick-string-lookup algorithm that finds the optimal match. Deferred. |
