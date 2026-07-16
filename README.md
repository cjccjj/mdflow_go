# mdflow

Streaming Markdown-to-ANSI renderer for the terminal. Built for AI output pipelines — renders as bytes arrive, works across chunk boundaries, never buffers the full document. Not a full-featured reader like [glow](https://github.com/charmbracelet/glow) / [glamour](https://github.com/charmbracelet/glamour).

## Install

Binary download (linux amd64 / arm64):
```bash
curl -sSfL https://raw.githubusercontent.com/cjccjj/mdflow/main/install.sh | sh
```
Or pick a binary from [releases](https://github.com/cjccjj/mdflow/releases).

Go toolchain (any platform):
```bash
go install github.com/cjccjj/mdflow/cmd/mdflow@latest
```

## Usage

**Pipe** (stdin → stdout):
```bash
echo '# Hello **world**' | mdflow
llm "explain goroutines" | mdflow
```

**Go library**:
```go
import "github.com/cjccjj/mdflow/pkg/markdown"

r := markdown.NewRenderer(os.Stdout)
r.Write([]byte("**bold** and *italic*"))
r.Close()
```

## Capability contract

mdflow supports the terminal-oriented subset below. “Supported” means it has
exact parser-event contract coverage and streaming-boundary tests; it does not
mean byte-for-byte CommonMark HTML conformance.

| Capability | Status | Notes |
|---|---|---|
| ATX and setext headings | Supported | `#`–`######`, `===`, and `---` |
| Emphasis | Supported subset | `*`, `_`, `**`, `__`, `~~`; common nesting and balanced repeated strong runs are covered |
| Inline and block code | Supported subset | Inline spans plus fenced and indented blocks; rare whitespace rules remain partial |
| Flat lists | Supported subset | `-`, `*`, ordered markers, and empty items; nested/continuation list structure is deferred |
| Blockquotes and thematic breaks | Supported subset | Common one-line and streaming forms |
| Links, images, and autolinks | Supported subset | Inline links/images and URI/email autolinks; reference links are not globally resolved |
| Escapes and entities | Supported subset | Common backslash escapes and HTML entity decoding |
| HTML | Terminal-specific | Tags are stripped and visible text is retained; raw HTML semantics are intentionally not preserved |
| GFM tables and strikethrough | Supported subset | Tables render with terminal redraw support |

Deferred: nested lists and list continuations, global reference-link
resolution, the full CommonMark delimiter algorithm, task lists, bare URL
linkification, syntax highlighting, and full raw-HTML fidelity. See
[Streaming_Limitations.md](dev_docs/Streaming_Limitations.md) for the
streaming rationale and concrete boundary cases.

## CommonMark compatibility diagnostics

The full CommonMark source fixture is smoke-tested by `make test`. A separate,
opt-in diagnostic compares only event semantics that mdflow can represent; it
does not compare ANSI output to expected HTML.

```bash
# Inspect one fixture with its first semantic divergence and parser trace.
go run -tags diagnose ./cmd/mdflow-diagnose -example 391

# Regenerate the compact full-fixture inventory.
go run -tags diagnose ./cmd/mdflow-diagnose \
  -write-inventory test/commonmark_baseline.json

# Compare a new run with a saved inventory.
go run -tags diagnose ./cmd/mdflow-diagnose \
  -baseline test/commonmark_baseline.json
```

The diagnostic trace is build-tagged and internal-only; it does not change the
public renderer or parser APIs. The committed inventory ranks compatibility
work and reports deltas, but only mdflow-owned core and promoted regression
cases are mandatory gates.

## API

```go
import (
    "github.com/cjccjj/mdflow/pkg/markdown"
    "github.com/cjccjj/mdflow/pkg/markdown/render"
)

r := markdown.NewRenderer(os.Stdout)

r.Write(data)   // parse & render chunk
r.Flush()       // drain buffered tokens (keep state)
r.Reset()       // reset for a new document
r.Close()       // flush + close open styles + reset

// Custom theme
t := render.DefaultTheme
t.Bold = render.Style{Prefix: "\033[1;31m", Suffix: "\033[0m"}
r := markdown.NewRenderer(os.Stdout, markdown.WithTheme(t))
```
