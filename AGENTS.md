# AGENTS.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

mdflow is a streaming Markdown-to-ANSI renderer for the terminal, built for AI output pipelines. It renders as bytes arrive, works across chunk boundaries, and never buffers the full document. Go 1.25 module at `github.com/cjccjj/mdflow`.

## Commands

```bash
make build              # build binary → ./mdflow
make test               # go test ./...
make lint               # golangci-lint run
make fmt                # gofmt -w .
make build-all          # cross-compile linux/amd64 + linux/arm64
```

Run a single test or package:
```bash
go test ./pkg/markdown/... -run TestName -v
go test ./pkg/markdown/render/... -v
```

The spec conformance test reads `dev_docs/commonMark_spec.txt`. Run it with:
```bash
go test ./pkg/markdown/... -run TestSpec -v
```

## Architecture

Three-layer streaming pipeline: **Tokenizer → Parser → Writer**

```
io.Writer → Renderer → Pipeline → Tokenizer.Tokenize(line) → Parser.Parse(tokens) → Writer.Handle(events) → AnsiWriter → os.Stdout
```

### 1. Tokenizer (`pkg/markdown/tokenizer/`)

Converts raw bytes into typed tokens (`TextToken`, `StarToken`, `BacktickToken`, `NewlineToken`, etc.). Pure lexical scan — no grammatical context. Runs once per newline-delimited chunk.

### 2. Parser (`pkg/markdown/parser/`)

State-machine parser that consumes tokens from a buffer and emits semantic `Event` values. The parser can pause mid-stream: if the buffer doesn't contain enough tokens to decide, it waits for more input.

**13 parser states** (`state.go`): `NormalState`, `HeaderState`, `InlineCodeState`, `CodeBlockState`, `IndentedCodeBlockState`, `BlockquoteState`, `TablePendingState`, `TableBodyState`, `SetextPendingState`, `LinkTextState`, `LinkURLState`, `HTMLBlockState`, `LinkRefDefState`.

`BoldState`/`ItalicState`/`StrikethroughState` are used only as emphasis stack frame labels, not as parser states — emphasis is handled within `NormalState` by `emphasisParser`.

**Sub-parser architecture** — the `Parser` struct delegates feature-specific work to 7 focused sub-parsers. Each owns its state, holds a `*Parser` back-pointer for shared buffer/line-context access, and exposes a `reset()` method:

| Sub-parser | File | Owns | Key methods |
|---|---|---|---|
| `linkParser` | `link_parser.go` | `linkBuf`, `linkURLBuf`, `linkDepth`, `urlParenDepth`, `urlAngleBracket`, `urlDone`, `urlHadNewline`, `linkTitleBuf`, `linkBracketConsumed`, `isImage` | `processLinkText()`, `processLinkURL()`, `flushLinkAsText()`, `emitLinkEvent()`, `collectTitle()`, `startImageText()` |
| `emphasisParser` | `emphasis.go` | `emphStack []emphasisFrame` | `tryStar()`, `tryUnderscore()`, `tryTilde()`, `tryCloser()`, `tryBulletOrBold()`, `drain()` |
| `tableParser` | `tables.go` | `tableHeaderBuf`, `tableColWidths`, `tableColAligns` | `tryTableHeader()`, `processTablePending()`, `processTableBody()`, `readTableCells()` |
| `blockParser` | `block_parser.go` | `headerLvl`, `fenceLen`, `fenceChar`, `codeBlockFirst`, `codeBlockIndent`, `blockquoteHadBlank` | (state only; block methods still on `*Parser` in `blocks.go`) |
| `setextParser` | `setext_parser.go` | `setextWaiting`, `setextBuf` | `flushSetext()` |
| `htmlBlockParser` | `html_block_parser.go` | `htmlBlockType`, `htmlIndent` | (state only) |
| `linkRefDefParser` | `link_ref_def_parser.go` | `lrdWaiting`, `lrdBuf` | `tryLinkRefDef()`, `processLinkRefDef()`, `flushLinkRefDef()` |

**Shared state on Parser** (cross-cutting, not feature-specific):

| Embedded type | Fields |
|---|---|
| `tokenBuffer` | `buf []Token`, `eof bool` |
| `lineContext` | `lineStart bool`, `prevChar byte`, `contentIndent int` |

**Source file map:**

| File | Responsibility |
|---|---|
| `parser.go` | Core: `Parser` struct, `New()`/`Reset()`/`Parse()`, `process()` dispatch loop, `processNormal()` with ordered dispatch stages, `emitTextOrSpecial()`, `processBacktickStart()`, `handleIndentedList()`, `tryImage()`, `tryAutolink()` with URI/email validation |
| `blocks.go` | Block-level methods (still on `*Parser`, accessing sub-parser state): ATX/setext headings, fenced/indented code blocks, blockquotes, thematic breaks, lists, HTML blocks. ~1175 lines (was ~1470 before refactor). |
| `inlines.go` | `processInlineCode()` — inline code span processing. Link and emphasis methods moved to sub-parsers; dead Bold/Italic/Strikethrough handlers removed. ~150 lines (was ~665). |
| `emphasis.go` | `emphasisParser` type, stack ops (`push`/`pop`/`top`/`drain`), flanking predicates (`canUnderscoreOpen`/`canUnderscoreClose`), `tryStar()`/`tryUnderscore()`/`tryTilde()`/`tryCloser()`, `tryBulletOrBold()` |
| `tables.go` | `tableParser` type, GFM table parsing: `readTableCells()`, `parseSeparatorAligns()`, `normalizeTableCells()` |
| `link_parser.go` | `linkParser` type, inline links `[text](url)`, reference links `[text][label]`, images `![alt](url)` |
| `link_ref_def_parser.go` | `linkRefDefParser` type, `parseLinkRefDefLine()`, `findUnescapedBracket()` |
| `setext_parser.go` | `setextParser` type, `flushSetext()` |
| `html_block_parser.go` | `htmlBlockParser` type (state only) |
| `block_parser.go` | `blockParser` type (state only) |
| `close.go` | `CloseStates()`, `Flush()`, `safeFlush()`, `finalizeState()` — per-state switch delegates to sub-parsers |
| `escapes.go` | Backslash escapes, hard line breaks, `hasMatchingCloser()`/`hasFlankingCloser()` helpers |
| `entities.go` | HTML entity decoding, `resolveEntities()` |
| `recognizers.go` | Thin wrappers adapting sub-parser methods to the `Recognizer` signature for `processNormal()` dispatch |
| `helpers.go` | `orderedListPrefix()`, `tabRemainingEquiv()`, indent ops, flanking checks (`isLeftFlankingRun`/`isRightFlankingRun`), whitespace helpers |
| `events.go` | `EventType` enum and `Event` struct — includes `ImageEvent`, `ImageRefEvent`, `AutolinkURLEvent`, `AutolinkEmailEvent` |
| `state.go` | `State` enum |

**The process loop** (`parser.go:process()`): iterates while tokens are available, dispatching to the current state's handler (which may delegate to a sub-parser). Breaks if no progress was made (state unchanged + buffer not consumed), waiting for more input.

**The dispatch order** (`parser.go:processNormal()`):
1. `processEscapeOrEntity()` — backslash escapes and `&` entities
2. `processLineStartBlock()` — thematic breaks, ATX headings, blockquotes, bullets, ordered lists, fenced code, HTML blocks, link ref defs *(line-start only)*
3. `processInlineStart()` — backtick (inline code / fenced code block), `[` (link), `~`/`*`/`_` (emphasis) *(always)*
4. `tryImage()` — `![` detection, delegates to `linkParser.startImageText()` *(always)*
5. `tryAutolink()` — `<URI>` and `<email>` autolink detection with URI/email validation *(always)*
6. `processDeferredLineStart()` — tables, indented code, setext candidates *(line-start only)*
7. `emitTextOrSpecial()` — fallback: emit plain text, checking `bufferHasPattern()` at each step for early break

### 3. Writer (`pkg/markdown/render/`)

Converts parser `Event` values to ANSI terminal output via `AnsiWriter`.

| File | Responsibility |
|---|---|
| `writer.go` | `Handle(Event)` — maps each event type to ANSI output. Manages emphasis SGR code composition for nested bold/italic/strikethrough |
| `ansi.go` | `AnsiWriter` — thin wrapper over `io.Writer` with `WriteStyled(text, Style)` |
| `styles.go` | `Theme` struct (27 `Style` fields) and `DefaultTheme` with ANSI escape codes |
| `table.go` | Full table rendering with live redraw: uses `\033[nA` cursor-up codes to repaint tables when column widths change as more rows arrive. Caps repaints at 50/table to bound cost |
| `tty.go` | `IsTerminal(w)` and `TerminalWidth(w)` — terminal detection via `golang.org/x/term` |
| `util.go` | `VisibleLen` (ANSI-aware string width), `RenderInline` (one-shot inline parse+render for table cells), `WrapContent` (width-aware line wrapping preserving ANSI codes) |

### Pipeline (`pkg/markdown/pipeline.go`)

The `Pipeline` struct connects all three layers:
- Splits input at `\n` boundaries
- Handles UTF-8 partial byte sequences at chunk boundaries (saves incomplete bytes in `utf8Buf`)
- Feeds each line through Tokenize → Parse → Handle
- `Flush()` drains buffered parser state without closing open constructs
- `Close()` drains UTF-8 buffer, flushes parser, then calls `CloseStates()` to emit end events for any open constructs

### Public API (`pkg/markdown/renderer.go`)

```go
r := markdown.NewRenderer(os.Stdout)
r := markdown.NewRenderer(os.Stdout, markdown.WithTheme(customTheme))
r.Write(data)   // parse & render chunk (any byte boundary)
r.Flush()       // drain buffered tokens (keeps parser state)
r.Reset()       // reset for a new document
r.Close()       // flush + close open styles + reset
```

## Testing

- `pkg/markdown/golden_test.go` — 39 golden cases comparing one-shot vs line-chunked vs arbitrary-split rendering; verifies streaming yields same output as whole-document rendering
- `pkg/markdown/commonmark_spec_test.go` — parses 652 CommonMark spec examples from `dev_docs/commonMark_spec.txt`; verifies no-crash and text preservation
- `pkg/markdown/parser/parser_test.go` — unit tests for parser states, token→event transformation, separator alignment
- `pkg/markdown/parser/characterization_test.go` — streaming boundary tests for flush/close behavior across chunks
- `pkg/markdown/parser/link_parser_test.go` — 41 tests: inline links, reference links, balanced brackets, streaming, titles, images, edge cases
- `pkg/markdown/parser/autolink_parser_test.go` — 24 tests: URI autolinks, email autolinks, validation, streaming, edge cases
- `pkg/markdown/parser/emphasis_parser_test.go` — 24 tests: bold/italic/strikethrough, nesting, flanking, intraword, multiple-of-3, streaming, close drain
- `pkg/markdown/parser/table_parser_test.go` — 11 tests: table detection, alignments, multiple rows, streaming, flush, edge cases
- `pkg/markdown/render/*_test.go` — unit tests for writer, styles, ANSI output, table rendering
- `pkg/markdown/robustness_test.go` — edge-case, streaming, large input, and custom theme tests

## Key design constraints

- **Never buffers the full document** — processes line-by-line. Some features (shortcut reference links, nested structures) are deliberately limited or omitted because they require lookahead or full-document context.
- **Parser states can pause** — if a state handler doesn't consume tokens and doesn't change state, the loop breaks and waits for more input. This is how streaming works across chunk boundaries.
- **EOF-awareness** — the parser has an `eof` flag on the token buffer. States use it to decide "no more data coming, resolve with what we have."
- **Recovery on panic** — `Parse()` recovers from panics via `safeFlush()`, emitting remaining buffered tokens as text and resetting state, so a bug in one chunk doesn't break the entire stream.
- **Tables repaint in place** — in a live terminal, tables redraw as column widths grow. The writer tracks how many lines it emitted and uses ANSI cursor-up codes to overwrite. Repaints are capped at 50 per table session.

## Adding new features

Follow the sub-parser pattern established by the refactor. See `dev_docs/Adding_Features.md` for detailed guidance. In brief:

1. **New inline construct** (e.g., `==highlight==`): create a `highlightParser` with its own state fields, a `*Parser` back-pointer, a `reset()` method, and `tryHighlight()` entry point. Register it in `processInlineStart()`. Add start/end events. Handle in `Writer.Handle()`. Add a `finalizeState` case in `close.go`.

2. **New block construct**: create a `blockParser` sub-type (or add a new sub-parser). Register in `processLineStartBlock()` or `processDeferredLineStart()`. Add a parser state if the construct spans multiple lines. Handle start/end events in the writer.

3. **New emphasis-like construct**: extend `emphasisParser` — add a new frame state, a new opener function following the `tryStar`/`tryUnderscore`/`tryTilde` pattern, register in `processInlineStart()`, handle in `enterEmphasis`/`exitEmphasis` for ANSI composition.

4. **Always**: add tests in the sub-parser's test file, wire `Reset()` to call the sub-parser's `reset()`, update `finalizeState` in `close.go`, and ensure streaming across chunk boundaries works (test with `Parse()` called in multiple chunks).

## Role of CommonMark spec

CommonMark is a reference and robustness benchmark, not a strict compliance target.

`dev_docs/commonMark_spec.txt` contains the spec and examples. It is long and sectioned, so consult only the relevant parts for the feature being worked on.

Key points:

1. **Output differs** — CommonMark defines HTML output; mdflow renders ANSI terminal output, so expected results are not directly comparable.

2. **Architecture differs** — CommonMark assumes full-document parsing; mdflow is a streaming state machine that cannot buffer the whole document.

3. **Streaming comes first** — When strict CommonMark behavior conflicts with incremental rendering, prefer predictable streaming behavior.

4. **Tests are for robustness** — `commonmark_spec_test.go` uses the spec examples to check no-crash behavior and text preservation, not full CommonMark conformance.