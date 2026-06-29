# Adding Features to mdflow

This guide covers how to add new Markdown constructs, extend existing ones, and debug the
streaming parser. It reflects the post-refactor architecture (2026-06-28) where the `Parser`
struct delegates to 7 focused sub-parsers.

---

## Architecture Overview

```
Input bytes
  │
  ├─ Pipeline.Write(data)
  │   ├─ UTF-8 boundary fixup
  │   ├─ Split on '\n' → chunks
  │   └─ For each chunk:
  │       ├─ tokenizer.Tokenize(chunk) → []Token
  │       ├─ parser.Parse(tokens) → []Event
  │       └─ writer.Handle(event) → ANSI bytes
  │
  └─ Pipeline.Close()
      ├─ Flush UTF-8 buf
      ├─ parser.CloseStates() → close open spans
      └─ writer.Handle(event) → ANSI bytes
```

### Parser sub-system (`pkg/markdown/parser/`)

```
Parser (lean: state + dispatch + back-pointers)
├── linkParser         — inline links [text](url), reference links [text][label]
├── emphasisParser     — bold/italic/strikethrough with 3-frame stack
├── tableParser        — GFM table detection, cell extraction, alignment
├── blockParser        — headings, code blocks, blockquotes, lists state
├── setextParser       — setext heading 1-line lookahead
├── htmlBlockParser    — HTML block type tracking
└── linkRefDefParser   — [label]: url definition lines
```

### Adding a feature: checklist

Every new feature touches these layers:

| Layer | What to do | Files |
|---|---|---|
| **Tokenizer** | Add a token type if the syntax uses a new special character | `tokenizer/token.go`, `tokenizer/tokenizer.go` |
| **Parser dispatch** | Register the feature in the right dispatch stage | `parser/parser.go` (`processNormal` or its delegates) |
| **Parser state** | Add a new `State` if the construct spans multiple tokens/lines | `parser/state.go` |
| **Sub-parser** | Create a focused type with state fields, back-pointer, `reset()` | new file in `parser/` |
| **Events** | Add start/end event types (or reuse existing ones) | `parser/events.go` |
| **Close/Flush** | Add a case in `finalizeState` for the new state | `parser/close.go` |
| **Writer** | Handle the new events → ANSI output | `render/writer.go` |
| **Theme** | Add style fields if the feature needs visual styling | `render/styles.go` |
| **Tests** | Sub-parser tests, golden tests, streaming boundary tests | `parser/`, `pkg/markdown/` |
| **Reset** | Call the sub-parser's `reset()` in `Parser.Reset()` | `parser/parser.go` |

---

## Pattern 1: New Inline Construct

Example: adding `==highlight==` (custom inline span).

### Step 1: Tokenizer

If the syntax uses `=`, add `EqualToken` to `tokenizer/token.go`:
```go
EqualToken  // =
```
And handle `'='` in `tokenizer/tokenizer.go`:
```go
case '=':
    flushText()
    tokens = append(tokens, Token{Type: EqualToken, Value: "="})
```

### Step 2: Sub-parser

Create `parser/highlight_parser.go`:
```go
type highlightParser struct {
    fenceLen int
    p        *Parser
}

func newHighlightParser(p *Parser) *highlightParser { return &highlightParser{p: p} }
func (hp *highlightParser) reset() { hp.fenceLen = 0 }

func (hp *highlightParser) tryHighlight() ([]Event, bool) {
    p := hp.p
    count := p.countConsecutive(tokenizer.EqualToken)
    if count < 2 {
        return nil, false
    }
    // Check for matching closer
    if !hasMatchingCloser(p.buf[count:], tokenizer.EqualToken, count, false) {
        p.consume(count)
        return []Event{{Type: TextEvent, Value: strings.Repeat("=", count)}}, true
    }
    p.consume(count)
    hp.fenceLen = count
    p.state = HighlightState  // new state
    return []Event{{Type: HighlightStartEvent}}, true
}

func (hp *highlightParser) processHighlight() []Event {
    p := hp.p
    // Look for closing `==`
    for len(p.buf) > 0 {
        matched, waiting := p.checkConsecutive(tokenizer.EqualToken, hp.fenceLen)
        if matched { ... emit HighlightEndEvent, return }
        if waiting { break }
        // emit text
    }
    return nil
}
```

### Step 3: Wire into dispatch

In `parser/parser.go`, `processInlineStart()`:
```go
if first.Type == tokenizer.EqualToken {
    return p.highlightParser.tryHighlight()
}
```

In `process()`:
```go
case HighlightState:
    events = append(events, p.highlightParser.processHighlight()...)
```

### Step 4: Events

Add to `parser/events.go`:
```go
HighlightStartEvent
HighlightEndEvent
```

### Step 5: Writer

Add to `render/writer.go` `Handle()`:
```go
case parser.HighlightStartEvent:
    _, err := w.aw.WriteString(w.theme.Highlight.Prefix)
    return err
case parser.HighlightEndEvent:
    _, err := w.aw.WriteString(w.theme.Highlight.Suffix)
    return err
```

### Step 6: Close/Flush

In `parser/close.go`, `finalizeState()`:
```go
case HighlightState:
    if mode != finalizeFlush {
        out = append(out, Event{Type: HighlightEndEvent})
    }
```

### Step 7: Reset

In `parser/parser.go`, `Reset()`:
```go
p.highlightParser.reset()
```

---

## Pattern 2: New Block Construct

Example: adding `::: callout` (fenced div).

### Key difference from inline

Block constructs:
- Are recognized at line start (`p.lineStart == true`)
- Typically span multiple lines until a closing condition
- Need a state in `state.go`
- Register in `processLineStartBlock()` or `processDeferredLineStart()`

### Dispatch registration

In `parser/parser.go`, `processLineStartBlock()`:
```go
if first.Type == tokenizer.TextToken && strings.HasPrefix(first.Value, ":::") {
    return p.calloutParser.tryCallout(), true
}
```

### State machine

A block construct state handler returns events for each line. When the closing condition
is met, it emits an end event and transitions back to `NormalState`. While active, the
parser stays in the block's state and the `process()` loop dispatches to it each time
new tokens arrive.

---

## Pattern 3: New Emphasis-Like Construct

Example: adding `^^superscript^^`.

Emphasis constructs use the **emphasis stack** because they can overlap with bold/italic
on the same text span. The terminal requires combined ANSI SGR codes for overlapping
styles.

### Steps

1. Add a new `State` (e.g., `SuperscriptState`) — used as a frame label, NOT as a parser state
2. Add a new `EventType` pair (e.g., `SuperscriptStartEvent`, `SuperscriptEndEvent`)
3. In `emphasis.go`, add `trySuperscript()` following the `tryTilde()` pattern
4. Register in `processInlineStart()`: `EqualToken` → `p.emphasisParser.trySuperscript()`
5. In `Writer`, add `inSuperscript bool` flag, update `enterEmphasis`/`exitEmphasis` SGR composition
6. In `emphasisEndEvent`/`emphasisStartEvent`, add the `SuperscriptState` case
7. Add to theme: `Superscript Style`

---

## Streaming Considerations

### When the parser must wait

The parser suspends when it sees an ambiguous prefix but lacks enough tokens to decide.
This is the `waiting` return from `checkConsecutive()`:

```go
matched, waiting := p.checkConsecutive(tokenizer.StarToken, 2)
if waiting {
    return nil, true  // "handled, but need more tokens — retry next chunk"
}
```

The `process()` loop detects "no progress" (buffer unchanged, state unchanged) and breaks,
waiting for the next `Parse()` call with more tokens.

### Cross-chunk state

Sub-parser state persists across `Parse()` calls. For example, `linkParser.linkBuf`
accumulates link text tokens across chunk boundaries. This works because sub-parsers
are long-lived (created once in `New()`, reset only in `Reset()`).

### EOF handling

When `p.eof` is true, ambiguous sequences must resolve. The parser should:
- Emit literal text for incomplete constructs (e.g., unclosed `**`)
- Close open emphasis spans via `CloseStates()` → `drain()`

### Bounded lookahead

Some features buffer one or two lines before emitting:
- **Setext headings**: buffer the candidate line, wait for `===`/`---` on next line
- **Tables**: buffer header row, validate separator row exists
- **Link reference defs**: may buffer multiple continuation lines until a valid definition or blank line

Buffering strategy: store tokens in sub-parser fields (like `setextBuf`, `lrdBuf`).
When the lookahead resolves, emit all events at once. When it fails (e.g., no underline
follows), emit the buffered content as plain text.

---

## Debugging

### Common failure modes

**"Feature not detected"** — check dispatch order in `processNormal()`. Earlier stages
take priority. For example, backtick and `[` are checked before `*`/`_` per Rule 17.

**"State stuck"** — the parser is in the wrong state and can't exit. Check the state's
exit conditions: does it handle newlines? EOF? Block-level tokens? Add `p.eof` guards.

**"Events out of order"** — start/end events must be balanced. Use `close.go`'s
`finalizeState` to emit missing end events on close/flush.

**"Streaming produces different output than one-shot"** — a chunk boundary splits a
multi-token pattern. Check `checkConsecutive` returns `waiting=true` correctly.
Verify sub-parser state persists across `Parse()` calls.

**"Content lost"** — `safeFlush()` converts buffered tokens to text on panic. Check
the panic site; the parser should never drop content. If it does, add a `recover`
path or make the ambiguous case emit literal text.

### Useful test techniques

1. **One-shot vs streaming golden tests**: use `renderOneShot()` and `renderSplitAt()`
   from `golden_test.go` to compare. Split at every byte position to find the exact
   boundary that breaks.

2. **Event-level characterization**: test the parser directly (not the full pipeline)
   to verify event sequences:
   ```go
   p := parser.New()
   events := p.Parse(tokenizer.Tokenize([]byte("**bold**")))
   ```

3. **Streaming simulation**: use `test/stream.sh` to pipe input in random-sized chunks
   with delays, simulating real AI output.

4. **AI evaluation**: `test/ai_test.py run` compares mdflow output against glow via
   LLM judge (scores content/structure/style 0-3).

### Adding CommonMark spec examples

The spec file is at `dev_docs/commonMark_spec.txt`. Test examples are fenced with
`` `````````````````````````````````` example ``. Each example has markdown (before `.`)
and expected HTML (after `.`). The Go test `TestCommonMarkSpec` verifies no-crash and
text preservation for each example.

---

## Tradeoff Guidelines

When deciding how to handle a Markdown feature that doesn't fit mdflow's streaming model:

### Use small bounded lookahead (1-2 lines)

For features with clear start markers and predictable end conditions where the lookahead
is small and the feature is valuable. Examples: setext headings, tables.

**Implementation**: buffer tokens in sub-parser fields, validate on the next line,
emit all events when confirmed, or flush as text when rejected.

### Simplify features requiring long-distance lookup

For features that need information from elsewhere in the document (link reference
definitions, shortcut reference links). These don't fit the streaming model.

**Implementation**: render the visible content as-is, add a visible hint (e.g.,
`[→ label]` for reference links), avoid full-document state.

### Support nested state only when valuable

For constructs that can overlap with other inline styles on the same characters.
The terminal can compose ANSI SGR codes, so limited stacking is feasible.

**Implementation**: use the emphasis stack pattern (max 3 frames). Use visible
text markers for deeper nesting or structural nesting (blockquote-in-list, etc.).

### Prefer visible markers over complex ANSI

When ANSI cannot represent a structure well (e.g., nested containers, multi-level
blockquotes), use visible text markers that the human reader can interpret directly.
Examples: `│ ` for blockquote, `• ` for bullet, `## ` for heading.

---

## File Reference

| File | Lines | Purpose |
|---|---|---|
| `parser/parser.go` | ~480 | Parser struct, dispatch, New/Reset/Parse |
| `parser/blocks.go` | ~1175 | Block-level methods (headings, code, blockquote, lists) |
| `parser/inlines.go` | ~150 | `processInlineCode()` only |
| `parser/emphasis.go` | ~420 | `emphasisParser`, stack, star/underscore/tilde |
| `parser/tables.go` | ~210 | `tableParser`, cell scanning, alignment |
| `parser/link_parser.go` | ~550 | `linkParser`, inline links, reference links, titles |
| `parser/link_ref_def_parser.go` | ~230 | `linkRefDefParser`, LRD line parsing |
| `parser/setext_parser.go` | ~40 | `setextParser`, flushSetext |
| `parser/html_block_parser.go` | ~25 | `htmlBlockParser` (state only) |
| `parser/block_parser.go` | ~35 | `blockParser` (state only) |
| `parser/close.go` | ~115 | Flush, CloseStates, finalizeState |
| `parser/helpers.go` | ~450 | Flanking checks, whitespace, indent, token buffer ops |
| `parser/escapes.go` | ~160 | Backslash escapes, closer detection helpers |
| `parser/entities.go` | ~240 | Entity decoding |
| `parser/recognizers.go` | ~240 | Thin wrappers for processNormal dispatch |
| `parser/events.go` | ~105 | Event types and struct |
| `parser/state.go` | ~22 | State enum |
| `render/writer.go` | ~365 | Event→ANSI rendering |
| `render/table.go` | ~430 | Table live repaint, width limiting, wrapping |
| `render/styles.go` | ~115 | Theme, DefaultTheme |
| `pipeline.go` | ~115 | Pipeline orchestration, UTF-8 buffering |
