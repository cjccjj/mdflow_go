# AGENTS.md

This file provides guidance to Claude Code when working with code in this repository.

## Project

mdflow is a streaming Markdown-to-ANSI renderer for the terminal, built for AI output pipelines. It renders as bytes arrive, works across chunk boundaries, and never buffers the full document. Go 1.25 module at `github.com/cjccjj/mdflow`.

## Commands

```bash
make build              # build binary → ./mdflow
make test               # go test ./...
make lint               # golangci-lint run
make fmt                # gofmt -w .
```

```bash
go test ./pkg/markdown/... -run TestName -v    # run a specific test
```

**Diagnostic tool** (build tag `diagnose`):
```bash
go run -tags diagnose ./cmd/mdflow-diagnose/             # full spec inventory
go run -tags diagnose ./cmd/mdflow-diagnose/ --example=4 # single example
go run -tags diagnose ./cmd/mdflow-diagnose/ --json      # machine-readable output
```

---

## Architecture

Three-layer streaming pipeline: **Tokenizer → Parser → Writer**

```
io.Writer → Renderer → Pipeline → streamChunker → Tokenizer.Tokenize(chunk) → Parser.Parse(tokens) → []event.Event → Writer.Handle(event) → AnsiWriter → os.Stdout
```

### 1. Tokenizer (`pkg/markdown/tokenizer/`)

Converts raw bytes into typed tokens (`TextToken`, `StarToken`, `BacktickToken`, `NewlineToken`, etc.). Pure lexical scan — no grammatical context. Runs once per newline-delimited chunk.

### 2. Parser (`pkg/markdown/parser/`)

State-machine parser that consumes tokens from a buffer and emits canonical `event.Event` values. Can pause mid-stream: if the buffer doesn't contain enough tokens to decide, it waits for more input.

**13 parser states** (`state.go`): `NormalState`, `HeaderState`, `InlineCodeState`, `CodeBlockState`, `IndentedCodeBlockState`, `BlockquoteState`, `TablePendingState`, `TableBodyState`, `SetextPendingState`, `LinkTextState`, `LinkURLState`, `HTMLBlockState`, `LinkRefDefState`.

`BoldState`/`ItalicState`/`StrikethroughState` are used only as emphasis stack frame labels, not as parser states — emphasis is handled within `NormalState` by `emphasisParser`.

**Sub-parser architecture** — the `Parser` struct delegates feature-specific work to 7 focused sub-parsers:

| Sub-parser | File | Owns |
|---|---|---|
| `linkParser` | `link_parser.go` | Link/image text/URL/title buffers, bracket depth |
| `emphasisParser` | `emphasis.go` | `emphStack []emphasisFrame` for bold/italic/strikethrough |
| `tableParser` | `tables.go` | Table header, column widths, alignments |
| `blockParser` | `block_parser.go` | Heading level, fence info, code block indent |
| `setextParser` | `setext_parser.go` | Setext heading lookahead buffer |
| `htmlBlockParser` | `html_block_parser.go` | HTML block type and indent |
| `linkRefDefParser` | `link_ref_def_parser.go` | Link reference definition buffer |

**The dispatch order** (`parser.go:processNormal()`):
1. `processEscapeOrEntity()` — backslash escapes and `&` entities
2. `processLineStartBlock()` — thematic breaks, ATX headings, blockquotes, bullets, ordered lists, fenced code, HTML blocks, link ref defs *(line-start only)*
3. `processInlineStart()` — backtick, `[`, `~`/`*`/`_` *(always)*
4. `tryImage()` — `![` detection *(always)*
5. `tryAutolink()` — `<URI>` and `<email>` *(always)*
6. `tryInlineHTML()` — `<tag>` inline HTML detection *(always)*
7. `processDeferredLineStart()` — tables, indented code, setext candidates *(line-start only)*
8. `emitTextOrSpecial()` — fallback: emit plain text

### 3. Writer (`pkg/markdown/render/`)

Converts `event.Event` values to ANSI terminal output. Key files: `writer.go` (event→ANSI routing), `table.go` (live table redraw with cursor-up codes), `styles.go` (27 `Style` fields in `Theme`).

### Public API (`pkg/markdown/renderer.go`)

```go
r := markdown.NewRenderer(os.Stdout)
r.Write(data)   // parse & render chunk (any byte boundary)
r.Flush()       // drain buffered tokens (keeps parser state)
r.Reset()       // reset for a new document
r.Close()       // flush + close open styles + reset
```

---

## Example-driven CommonMark compatibility workflow

Development is example-driven because: Markdown has no formal grammar, the CommonMark spec itself is defined by 655 examples rather than rules, and mdflow operates as a streaming state machine with no AST — unlike the C reference implementation. So example-based projection comparison is the only tractable correctness check.

This workflow replaces ad-hoc patching with cluster-based fixes and regression protection.

### Diagnostic tool (`cmd/mdflow-diagnose/`, build tag `diagnose`)

The diagnostic tool runs projection-based comparison on all 655 CommonMark spec examples and reports the result for each:

- **match** — parser `[]Operation` stream equals HTML-projected `[]Operation` stream
- **mismatch** — first divergence found via `FirstDifference()`
- **noncomparable** — raw HTML, nested lists, reference links, GFM tables (intentionally excluded)

```bash
# Full spec inventory
go run -tags diagnose ./cmd/mdflow-diagnose/ --chunks=line

# Single example drill-down with trace
go run -tags diagnose ./cmd/mdflow-diagnose/ --example=4 --chunks=line

# JSON output for scripting
go run -tags diagnose ./cmd/mdflow-diagnose/ --json
```

### Semantic projection (`internal/commonmark/`)

Both CommonMark's expected HTML and the parser's event stream are projected into a canonical `[]Operation` stream:

- `ExpectedProjection(markdown, html)` — parses CommonMark HTML tags (`<em>`, `<strong>`, `<a>`, `<pre><code>`, etc.) into `[]Operation`
- `ActualProjection(parserEvents)` — maps `event.Event` → `[]Operation`
- `FirstDifference(expected, actual)` — returns first mismatch index and both operations, or nil if equal

Operation kinds: `text`, `newline`, `em_start`/`end`, `strong_start`/`end`, `strike_start`/`end`, `code_start`/`end`, `code_block_start`/`end`, `code_lang`, `heading_start`/`end`, `blockquote_start`/`end`, `list_item`, `thematic_break`, `link`, `image`.

### Clustering

Mismatches are grouped by normalized **transition signature**: `branch | pre_state | expected_kind | actual_kind | outcome`. Operation value is excluded from the signature to group related bugs together.

Largest current clusters are listed in the status table below, which also shows match/mismatch/noncomparable counts and protected regression test coverage per CommonMark section.

### Fixing a mismatch

**1. Analyze the landscape** — Use the Current status section overview (per-section table + cluster table) to identify high-value targets. Prioritize:
  - Sections with the most mismatches (emphasis 36, links 18, lists 23)
  - Clusters with structural parsing errors, not value normalization (e.g. emphasis-boundary 10, dash-as-bullet 10, star/bullet confusion 5)
  - Clusters where a single fix addresses many examples

**2. Select and triage** — Pick examples from the target cluster and review each manually:
  - `--example=N --chunks=line --trace` shows the divergence and surrounding state
  - Decide: **fix** (parser bug, clean transition fix), **defer** (needs deeper refactoring, keep in cluster), or **skip** (streaming limitation, record in `dev_docs/Streaming_Limitations.md`)
  - Start with the simplest example in the cluster

**3. Fix the example** — Per-example steps:
  1. **Minimize** (if not already minimal) — `--minimize` reduces input while preserving the divergence signature
  2. **Classify** — examine the root cause (e.g. missing state, incorrect transition, value normalization, future-dependence)
  3. **Fix** — adjust the parser (ideally one transition per fix)
  4. **Verify** — run the fixed example in all 3 streaming modes (one-shot, line, arbitrary-rune); run `--baseline` against the previous inventory to check for regressions across all 655 examples
  5. **Promote** (optional) — if the example is basic and representative, add it as a protected regression test via `assertProtectedSpecCases()`. Niche or edge-case fixes can remain covered by the smoke suite alone.

### Protected regression tests (`parser/commonmark_regression_test.go`)

Examples that are confirmed matches are promoted to permanent protected tests. Each runs in **3 streaming modes** (one-shot, line-boundary splits, per-rune splits) and verifies exact `[]Operation` equivalence via `FirstDifference()`. A future change that regresses a protected example must be corrected, expanded, or accepted as an explicit tradeoff.

---

## Testing

### Three-layer test strategy

| Layer | Test | Scope | Validates |
|---|---|---|---|
| **Smoke** | `TestCommonMarkSpec` | All 655 examples | No panic + text preservation |
| **Protected** | `TestProtected*` | 29 promoted examples | Full `[]Operation` equivalence across 3 streaming modes |
| **Debug** | `TestTraceRecorder` (`-tags diagnose`) | 4 synthetic inputs | Trace recorder doesn't change parser output |

### Diagnostic inventory (build tag `diagnose`)

Run full projection comparison on all 655 examples to get current match/mismatch/noncomparable counts, clustered by transition signature:
```bash
go run -tags diagnose ./cmd/mdflow-diagnose/ --chunks=line
```

Compare with a baseline to detect regressions:
```bash
go run -tags diagnose ./cmd/mdflow-diagnose/ --chunks=line --write-inventory=/tmp/before.json
# ... make changes ...
go run -tags diagnose ./cmd/mdflow-diagnose/ --chunks=line --baseline=/tmp/before.json
```

### Unit tests

- `pkg/markdown/golden_test.go` — 39 golden cases comparing one-shot vs line-chunked vs arbitrary-split rendering
- `pkg/markdown/parser/parser_test.go` — parser state and token→event tests
- `pkg/markdown/parser/link_parser_test.go` — 41 link/image tests
- `pkg/markdown/parser/emphasis_parser_test.go` — 24 emphasis tests
- `pkg/markdown/parser/inline_html_test.go` — 30 inline HTML tests
- `pkg/markdown/parser/table_parser_test.go` — 11 table tests
- `pkg/markdown/parser/autolink_parser_test.go` — 24 autolink tests
- `pkg/markdown/html_block_test.go` — 12 HTML block tests
- `pkg/markdown/parser/characterization_test.go` — streaming boundary tests
- `internal/commonmark/semantic_test.go` — projection parser unit tests

---

## Adding parser features

Current work is CommonMark spec compatibility fixes (see Fix workflow above). New Markdown constructs (e.g., GFM tables, strikethrough, task lists) will come later. When that time comes, follow the sub-parser patterns below and run the diagnostic pipeline as usual.

### Wiring a new construct

**New inline construct** (e.g., `==highlight==`):
- Create sub-parser with state fields, `*Parser` back-pointer, `reset()`
- Register in `processInlineStart()` 
- Add start/end `event` values
- Handle in `Writer.Handle()`
- Add `finalizeState` case in `close.go`

**New block construct**:
- Create sub-parser, register in `processLineStartBlock()` or `processDeferredLineStart()`
- Add parser `State` if spans multiple lines

**New emphasis-like construct**:
- Extend `emphasisParser` — add frame state, opener function following `tryStar` pattern
- Register in `processInlineStart()`

**Always**: wire `Reset()` to call sub-parser `reset()`, update `finalizeState`, test streaming across chunk boundaries.

---

## Key design constraints

- **Never buffers the full document** — only bounded input required for current constructs. Shortcut reference links, nested structures are limited/omitted.
- **Parser states can pause** — loop breaks when no progress; waits for more input.
- **Canonical event contract** — production communication uses `event.Event`. Keep `parser.Event` aliases for source compatibility.
- **EOF-awareness** — parser uses `eof` flag to resolve ambiguous sequences.
- **Recovery on panic** — `Parse()` recovers via `safeFlush()`, emitting remaining buffered tokens as text.
- **HTML stripped** — both block and inline HTML tags are stripped by parser; only visible text emitted.
- **Tables repaint in place** — uses ANSI cursor-up codes for live redraw, capped at 50 per table session.

## Role of CommonMark spec

`dev_docs/spec.json` (extracted from `commonMark_spec.txt`) contains the spec with 652 embedded examples. CommonMark is a reference and robustness benchmark, not a strict compliance target — mdflow is a streaming state machine that cannot buffer the full document.

- **Output differs** — CommonMark defines HTML; mdflow renders ANSI terminal output
- **Architecture differs** — CommonMark assumes full-document parsing; mdflow streams
- **Streaming comes first** — when strict CommonMark conflicts with incremental rendering, prefer predictable streaming behavior
- **Diagnostic, not compliance** — the semantic projection comparison identifies mismatches, but some are unavoidable under immediate output; those are recorded as streaming tradeoffs

---

## Current status

*Dynamic — update after each diagnostic inventory run.*

| Section | Protected | Match | Mismatch | Noncomp | Total |
|---|---|---|---|---|---|---|
| ATX headings | #63 | 17 | 1 | 0 | 18 |
| Autolinks | #597, #604 | 8 | 2 | 9 | 19 |
| Backslash escapes | #13 | 8 | 1 | 4 | 13 |
| Block quotes | #234 | 12 | 3 | 10 | 25 |
| Code spans | #339 | 11 | 7 | 4 | 22 |
| Emphasis | #350, #357, #389, #405, #417, #425, #465, #466, #468 | 94 | 31 | 7 | 132 |
| Entity references | #29 | 8 | 7 | 2 | 17 |
| Fenced code blocks | #126 | 20 | 8 | 1 | 29 |
| Hard line breaks | #644 | 7 | 6 | 2 | 15 |
| Images | #590 | 5 | 2 | 15 | 22 |
| Indented code blocks | #114 | 8 | 2 | 2 | 12 |
| Link reference defs | #209 | 4 | 5 | 18 | 27 |
| Links | #483 | 23 | 18 | 49 | 90 |
| List items / Lists | #280, #281, #295 | 31 | 10 | 33 | 74 |
| Paragraphs | #219 | 7 | 1 | 0 | 8 |
| Setext headings | #94 | 23 | 3 | 1 | 27 |
| Soft line breaks | #648 | 1 | 1 | 0 | 2 |
| Tabs | #1 | 7 | 0 | 4 | 11 |
| Textual content | #650 | 3 | 0 | 0 | 3 |
| Thematic breaks | #43 | 17 | 2 | 0 | 19 |
| Other (HTML, raw HTML) | — | 2 | 3 | 62 | 67 |
| **TOTAL** | **31** | **316** | **113** | **223** | **652** |

**Protected:** 31 spec examples across 24 test functions (every spec section covered except raw HTML). **Next priorities by volume:** emphasis (31 mismatches), links (18), setext headings (3).

### Largest mismatch clusters (40 total)

Mismatches grouped by transition signature (`branch | pre_state | expected_kind | actual_kind | outcome`). Operation value excluded to group related bugs.

| Count | Signature | Description | Example |
|---|---|---|---|
| 25 | `finalize.close \| normal \| text \| text` | Text-value diffs on finalize (entity encoding, whitespace) | #25 |
| 8 | `state.link_url \| link_url \| resource_link \| resource_link` | URL encoding differences in links | #489 |
| 6 | `finalize.flush \| normal \| text \| text` | Text-value diffs on flush (trailing whitespace, encoding) | #226 |
| 6 | `state.link_url \| link_url \| text \| resource_link` | Nested brackets in link URL | #342 |
| 6 | `normal.text \| normal \| text \| newline` | Extra blank lines emitted | #97 |
| 4 | `inline.star \| normal \| text \| emphasis_marker` | Star not recognized as emphasis at inline position | #341 |
| 4 | `finalize.close \| normal \| resource_link \| text` | Link broken on finalize (entity in URL, extra content) | #22 |
| 4 | `deferred.indented_code \| normal \| text \| code_block` | Indented code where none expected | #49 |
| 4 | `normal.text \| normal \| text \| text` | Plain text diffs (entity encoding, whitespace) | #376 |
| 4 | `finalize.flush \| normal \| text \| emphasis_marker` | Emphasis opened on flush | #438 |
| 3 | `state.code_block \| code_block \| text \| text` | Code block content diffs (encoding/whitespace) | #131 |
| 3 | `finalize.close \| normal \| code_start \| text` | Code span unclosed on finalize | #337 |
| 3 | `inline.underscore \| normal \| text \| text` | Underscore not recognized as emphasis | #398 |
| 3 | `line_start.bullet_or_emphasis_star \| normal \| text \| emphasis_marker` | Star at line start confused between bullet/emphasis | #444 |
