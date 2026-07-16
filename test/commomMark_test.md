# CommonMark Compatibility Matrix

This is mdflow’s concise capability contract, not a claim of byte-for-byte
CommonMark HTML conformance. The source fixture remains
`dev_docs/commonMark_spec.txt`; the diagnostic adapter compares only semantic
operations represented by `event.Event`.

| Status | Meaning |
|---|---|
| ✅ Supported subset | Exact mdflow event-contract and streaming tests cover representative behavior |
| ⚠️ Partial | Useful behavior exists, but edge rules or nesting differ |
| ↔ Terminal-specific | Intentional terminal semantic difference, not a parser failure |
| ⏭️ Deferred | Requires broader state, global knowledge, or is out of scope |

## CommonMark blocks and inlines

| Area | Status | mdflow contract |
|---|---:|---|
| Characters, line endings, NUL | ✅ | Streaming chunker and tokenizer preserve UTF-8 boundaries and normalize line endings |
| Tabs and indentation | ⚠️ | Common indentation works; detailed container indentation remains partial |
| Backslash escapes and entities | ✅ | Common punctuation escapes and named/numeric entities are decoded outside code |
| Thematic breaks | ✅ | `---`, `***`, `___`, including ordinary streaming boundaries |
| ATX/setext headings | ✅ | Levels, closing hashes, and common indented forms |
| Fenced and indented code | ⚠️ | Core forms supported; uncommon nesting/indentation rules are partial |
| Paragraphs and blank lines | ✅ | Text and line boundaries stream without document buffering |
| Blockquotes | ⚠️ | Common and lazy continuation forms work; deep nesting remains partial |
| Flat list items | ⚠️ | Bullet, ordered, and empty items work; continuation paragraphs and nested lists are deferred |
| Inline code spans | ⚠️ | Common delimiters work; all whitespace-normalization edge cases are not promised |
| Emphasis and strong emphasis | ⚠️ | Common forms, nesting, and balanced repeated strong runs work; full delimiter algorithm is deferred |
| Inline links and images | ⚠️ | Inline destinations and titles work; deeply nested/escaped edge cases remain partial |
| Reference links/definitions | ⏭️ | Parsed as local events, but not globally resolved across a document |
| Autolinks | ✅ | URI and email autolinks are rendered as links |
| Raw HTML | ↔ | Tags are stripped and visible text is preserved rather than rendering raw HTML |
| Hard and soft line breaks | ⚠️ | Terminal output uses visible line boundaries; HTML `<br>` parity is not the goal |

## GFM additions

| Feature | Status | mdflow contract |
|---|---:|---|
| Tables | ✅ | Header/separator/body rows with terminal redraw support |
| Strikethrough | ✅ | `~~text~~` participates in the emphasis stack |
| Task lists | ⏭️ | Render as ordinary list text |
| Bare URL linkification | ⏭️ | Only angle-bracket autolinks are recognized |

## Regression policy

`make test` runs all of the following:

- The full CommonMark no-panic/text-preservation smoke suite.
- mdflow-owned exact-event core contract cases.
- Existing renderer/golden/streaming tests.
- Protected compatibility regressions promoted from measured diagnostic clusters.

The opt-in `mdflow-diagnose` command writes a compact baseline inventory and
clusters only comparable semantic mismatches. Unsupported HTML, unsupported
nesting, and documented terminal/streaming tradeoffs are reported rather than
turned into blanket CI failures.

## Promoted compatibility regressions

| Cluster | Original spec examples | Protected variants |
|---|---|---|
| Balanced repeated strong delimiters | 391, 427, 467, 468, 470 | Minimal `____x____`, repeated star runs, whitespace/escape cases, line and arbitrary-rune splits |
| Empty list markers | 282, 283 | Minimal `-\n`, `*\n`, marker whitespace, escaped marker, line and arbitrary-rune splits |
