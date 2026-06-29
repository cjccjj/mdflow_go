# CommonMark Spec Coverage Examples

Spec version: CommonMark 0.31.2  
Purpose: concise examples for mdflow feature coverage.

## Legend

| Icon | Meaning |
|------|---------|
| ✅ | Supported |
| ⚠️ | Partial |
| ❌ | Not supported |
| ⏭️ | Skipped |
| 🔧 | In progress |

---

## 1. Introduction

Status: ⏭️ Skipped

Informational only. No Markdown cases to test.

---

## 2. Preliminaries

### 2.1 Characters and lines

Status: ✅

| Case | Input | Expected |
|---|---|---|
| LF line ending | `a\nb` | Two lines: `a`, `b` |
| CRLF line ending | `a\r\nb` | Two lines: `a`, `b` |
| CR line ending | `a\rb` | Two lines: `a`, `b` |

---

### 2.2 Tabs

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Tab tokenized | `a\tb` | Tab preserved as plain text spacing |
| Tab as code indentation | `\tcode` | Recognized as indented code block |
| Tab-based nesting | `-\titem` | ⚠️ Not fully CommonMark-accurate for indentation nesting |

---

### 2.3 Insecure characters

Status: ✅

| Case | Input | Expected |
|---|---|---|
| Null byte | `a\0b` | Rendered as `a�b` |

---

### 2.4 Backslash escapes

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Escape emphasis marker | `\*text*` | Literal `*text*` |
| Escape heading marker | `\# not heading` | Literal `# not heading`, not a heading |
| Escape backslash | `\\` | Literal `\` |
| Non-punctuation escape | `\a` | Literal `\a` |
| Escaped opener only | `\**bold*` | First `*` literal; remaining Markdown parsed normally |
| Escape inside code span | `` `\*not emphasis\*` `` | ⚠️ mdflow may consume escapes; CommonMark should keep them literal |
| Backslash hard break | `foo\` followed by newline | Newline rendered |

---

### 2.5 Entity and numeric character references

Status: ❌

| Case | Input | Expected |
|---|---|---|
| Named entity | `&amp;` | Raw text `&amp;` |
| Less-than entity | `&lt;` | Raw text `&lt;` |
| Numeric entity | `&#42;` | Raw text `&#42;` |

---

## 3. Blocks and inlines

Status: ⏭️ Skipped

Definitions and precedence rules only. No direct cases.

---

## 4. Leaf blocks

### 4.1 Thematic breaks

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Dash break | `---` | Thematic break |
| Star break | `***` | Thematic break |
| Underscore break | `___` | Thematic break |
| Interrupt paragraph | `text\n---` | Parsed as setext heading if directly after text; use blank line before HR |
| Mixed characters | `*-*` | Raw text, not thematic break |
| Different char types | `--*` | Raw text, not thematic break |
| Spaced markers | `- - -` | ⚠️ Not recognized |
| Trailing spaces | `---   ` | ⚠️ Not fully handled |
| Indented 0–3 spaces | `  ---` | ⚠️ Not fully handled |

---

### 4.2 ATX headings

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| H1 | `# Heading` | Level 1 heading |
| H2 | `## Heading` | Level 2 heading |
| H6 | `###### Heading` | Level 6 heading |
| Empty heading | `# ` | Empty level 1 heading |
| 7 hashes | `####### Heading` | Raw text, not heading |
| Missing space | `#Heading` | Raw text, not heading |
| Closing hashes | `## foo ##` | ⚠️ Rendered as heading text `foo ##`; CommonMark strips trailing `##` |
| Indented heading | `   # Heading` | ⚠️ Not fully handled |

---

### 4.3 Setext headings

Status: ✅

| Case | Input | Expected |
|---|---|---|
| H1 setext | `Title\n===` | Level 1 heading `Title` |
| H2 setext | `Title\n---` | Level 2 heading `Title` |
| Empty candidate | `\n---` | Thematic break or blank + break, not heading |
| HR after paragraph needs blank | `Title\n\n---` | Paragraph `Title`, then thematic break |
| Direct dash underline | `Title\n---` | Setext H2, not thematic break |

---

### 4.4 Indented code blocks

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Four spaces | `    code` | Indented code block containing `code` |
| Tab indentation | `\tcode` | Indented code block containing `code` |
| Blank line inside | `    a\n\n    b` | Code block preserves blank line |
| Trailing blank lines | `    code\n\n` | ⚠️ Trailing blank handling not fully spec-compliant |
| Lazy continuation | `    code\ncontinued` | ⚠️ Lazy continuation not fully handled |
| Inside lists | `- item\n    code` | ⚠️ List/code interaction incomplete |

---

### 4.5 Fenced code blocks

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Backtick fence | <code>```<br>code<br>```</code> | Code block |
| Tilde fence | <code>~~~<br>code<br>~~~</code> | Code block |
| Info string | <code>```go<br>fmt.Println("hi")<br>```</code> | Code block with language/info `go` |
| Longer closing fence | <code>```<br>code<br>````</code> | Code block closes correctly |
| Indented fence | <code>  ```<br>code<br>  ```</code> | ⚠️ 0–3 space indentation not fully handled |

---

### 4.6 HTML blocks

Status: ⏭️ Skipped

| Case | Input | Expected |
|---|---|---|
| Div block | `<div>\ntext\n</div>` | Passed through as raw text |
| HTML comment | `<!-- comment -->` | Passed through as raw text |
| Table HTML | `<table><tr><td>x</td></tr></table>` | Passed through as raw text |

---

### 4.7 Link reference definitions

Status: ⏭️ Skipped

| Case | Input | Expected |
|---|---|---|
| Reference definition | `[ref]: /url "title"` | Passed through as raw text |
| Later reference usage | `[text][ref]` | Not resolved; reference links unsupported |

---

### 4.8 Paragraphs

Status: ✅

| Case | Input | Expected |
|---|---|---|
| Simple paragraph | `hello world` | Paragraph text |
| Paragraph separation | `one\n\ntwo` | Two paragraphs |
| Paragraph interrupted by block | `text\n# Heading` | Paragraph followed by heading |

---

### 4.9 Blank lines

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Empty blank line | `a\n\nb` | Paragraph separation |
| Whitespace-only line | `a\n   \nb` | Paragraph separation |
| Blank line in list | `- a\n\n- b` | ⚠️ Tight/loose list distinction not implemented |

---

## 5. Container blocks

### 5.1 Block quotes

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Single line | `> quote` | Block quote rendered with quote prefix |
| Optional space | `>quote` | Block quote |
| Multiple lines | `> a\n> b` | Multi-line block quote |
| Nested quote | `> > nested` | ⚠️ Nested block quotes not fully handled |
| Lazy continuation | `> a\ncontinued` | ⚠️ Continuation line not included in quote |
| Indented quote | `   > quote` | ⚠️ 0–3 space indentation not fully handled |

---

### 5.2 List items

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Dash bullet | `- item` | Bullet list item |
| Star bullet | `* item` | Bullet list item |
| Ordered dot | `1. item` | Ordered list item |
| Ordered paren | `2) item` | Ordered list item |
| Mixed list markers | `- a\n1. b` | Rendered as flat list items |
| Nested sublist | `- a\n  - b` | ⚠️ Nesting not fully handled |
| Continuation paragraph | `- a\n  continued` | ⚠️ Continuation paragraph not fully handled |

---

### 5.3 Lists

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Flat bullet list | `- a\n- b` | Flat bullet list |
| Flat ordered list | `1. a\n2. b` | Flat ordered list |
| Loose list | `- a\n\n- b` | ⚠️ Tight/loose distinction not implemented |
| Nested list | `- a\n  - b` | ⚠️ Requires indentation/list stack |
| Continuation paragraph | `- a\n\n  more` | ⚠️ Not fully handled |

---

## 6. Inlines

### 6.1 Code spans

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Single backtick | `` `code` `` | Inline code `code` |
| Double backtick span | `` ``code ` inside`` `` | Inline code containing backtick |
| Formatting inside code | `` `**not bold**` `` | Literal `**not bold**` |
| No newline span | `` `a\nb` `` | Does not form a multi-line code span |
| Space normalization | `` ` code ` `` | ⚠️ Whitespace stripping not fully CommonMark-accurate |
| Spaces-only edge case | `` `   ` `` | ⚠️ Edge cases incomplete |

---

### 6.2 Emphasis and strong emphasis

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Bold stars | `**bold**` | Bold text |
| Italic stars | `*italic*` | Italic text |
| Bold underscores | `__bold__` | Bold text when valid mid-line |
| Italic underscores | `_italic_` | Italic text when valid mid-line |
| Strikethrough | `~~strike~~` | Struck-through text |
| Triple delimiter | `***bold italic***` | ⚠️ Full delimiter-run rules incomplete |
| Intraword star | `foo*bar*baz` | ⚠️ CommonMark edge cases incomplete |
| Intraword underscore | `foo_bar_baz` | ⚠️ Intraword underscore rules incomplete |
| Mod-3 rule | `***a** b*` | ⚠️ Full CommonMark algorithm not implemented |

---

### 6.3 Links

Status: ✅

| Case | Input | Expected |
|---|---|---|
| Inline link | `[text](https://example.com)` | Rendered link text plus URL |
| Link text only | `[text]` | Raw text or incomplete link |
| Reference full | `[text][ref]` | Unsupported; raw text |
| Reference shortcut | `[text]` with `[text]: /url` | Unsupported; raw text |
| Empty reference | `[text][]` | Unsupported; raw text |

---

### 6.4 Images

Status: ❌

| Case | Input | Expected |
|---|---|---|
| Inline image | `![alt](image.png)` | Passed through as raw text |
| Not mistaken for link | `![alt](image.png)` | Should not render as normal link |

---

### 6.5 Autolinks

Status: ❌

| Case | Input | Expected |
|---|---|---|
| URL autolink | `<https://example.com>` | Passed through as raw text |
| Email autolink | `<user@example.com>` | Passed through as raw text |

---

### 6.6 Raw HTML inline

Status: ⏭️ Skipped

| Case | Input | Expected |
|---|---|---|
| Inline tag | `<span>x</span>` | Passed through as raw text |
| HTML emphasis | `<em>x</em>` | Passed through as raw text |

---

### 6.7 Hard line breaks

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Backslash break | `foo\` followed by newline | Rendered newline |
| Two-space break | `foo  ` followed by newline | ⚠️ Not specially distinguished |
| CLI rendering | `foo\nbar` | Hard and soft breaks both render as visible newlines |

---

### 6.8 Soft line breaks

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Soft break | `foo\nbar` | Rendered as actual newline |
| HTML-style soft break | `foo\nbar` | ⚠️ Not rendered as space; CLI uses newline |

---

### 6.9 Textual content

Status: ⚠️ Partial

| Case | Input | Expected |
|---|---|---|
| Plain text | `hello world` | Plain text |
| Escaped punctuation | `\*literal\*` | Mostly handled |
| Entity text | `&amp;` | Not decoded |
| Unicode whitespace | `a b` | ⚠️ No special Unicode whitespace normalization |

---

## Appendix: A parsing strategy

Status: ⏭️ Skipped

Reference material only. No Markdown cases to test.

---

## Beyond CommonMark: GFM extensions

| Feature | Status | Input | Expected |
|---|---:|---|---|
| Tables | ✅ | `| A | B |\n|---|---|\n| 1 | 2 |` | Rendered table |
| Strikethrough | ✅ | `~~deleted~~` | Struck-through text |
| Task lists | ❌ | `- [ ] todo\n- [x] done` | Passed through as normal list text |
| Bare URLs | ❌ | `https://example.com` | Not auto-linked |
| CommonMark autolinks | ❌ | `<https://example.com>` | Passed through as raw text |