package commonmark

import (
	"fmt"
	"html"
	"strings"
	"unicode"

	"github.com/cjccjj/mdflow/pkg/markdown/event"
)

// Operation is the portable subset of the parser event contract used by the
// CommonMark diagnostic oracle. It intentionally describes semantics, not
// ANSI presentation or byte-for-byte HTML.
type Operation struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
	Level int    `json:"level,omitempty"`
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
}

func (o Operation) String() string {
	switch o.Kind {
	case "text":
		return fmt.Sprintf("text(%q)", o.Value)
	case "heading_start":
		return fmt.Sprintf("heading_start(%d)", o.Level)
	case "link", "image":
		if o.Title != "" {
			return fmt.Sprintf("%s(%q, %q, %q)", o.Kind, o.Value, o.URL, o.Title)
		}
		return fmt.Sprintf("%s(%q, %q)", o.Kind, o.Value, o.URL)
	case "list_item":
		return fmt.Sprintf("list_item(%s)", o.Value)
	default:
		return o.Kind
	}
}

// Projection contains either a comparable semantic stream or the reason the
// source falls outside the deliberately small test-only HTML adapter. Runtime
// parsing never depends on this representation.
type Projection struct {
	Operations []Operation `json:"operations,omitempty"`
	Comparable bool        `json:"comparable"`
	Reason     string      `json:"reason,omitempty"`
}

// ExpectedProjection parses CommonMark's expected HTML into semantic
// operations that can be represented by mdflow's event contract. Raw HTML,
// unsupported block nesting, and terminal-specific constructs are explicitly
// reported as non-comparable rather than treated as parser failures.
func ExpectedProjection(markdown, expectedHTML string) Projection {
	if containsRawHTML(markdown) {
		return Projection{Reason: "raw HTML is intentionally stripped by terminal semantics"}
	}

	p := htmlProjectionParser{input: expectedHTML}
	p.parse()
	if p.reason != "" {
		return Projection{Reason: p.reason}
	}
	return Projection{Operations: canonicalize(p.ops), Comparable: true}
}

// ActualProjection projects parser events into the same comparison subset.
// Extension-only or unresolved-reference events are intentionally excluded
// from the CommonMark oracle instead of producing misleading failures.
func ActualProjection(events []event.Event) Projection {
	ops := make([]Operation, 0, len(events))
	for _, e := range events {
		switch e.Type {
		case event.TextEvent:
			ops = append(ops, Operation{Kind: "text", Value: e.Value})
		case event.NewlineEvent:
			ops = append(ops, Operation{Kind: "newline"})
		case event.HeaderStartEvent:
			ops = append(ops, Operation{Kind: "heading_start", Level: e.Level})
		case event.HeaderEndEvent:
			ops = append(ops, Operation{Kind: "heading_end"})
		case event.BoldStartEvent:
			ops = append(ops, Operation{Kind: "strong_start"})
		case event.BoldEndEvent:
			ops = append(ops, Operation{Kind: "strong_end"})
		case event.ItalicStartEvent:
			ops = append(ops, Operation{Kind: "em_start"})
		case event.ItalicEndEvent:
			ops = append(ops, Operation{Kind: "em_end"})
		case event.StrikethroughStartEvent:
			ops = append(ops, Operation{Kind: "strike_start"})
		case event.StrikethroughEndEvent:
			ops = append(ops, Operation{Kind: "strike_end"})
		case event.InlineCodeStartEvent:
			ops = append(ops, Operation{Kind: "code_start"})
		case event.InlineCodeEndEvent:
			ops = append(ops, Operation{Kind: "code_end"})
		case event.CodeBlockStartEvent:
			ops = append(ops, Operation{Kind: "code_block_start"})
		case event.CodeBlockEndEvent:
			ops = append(ops, Operation{Kind: "code_block_end"})
		case event.CodeBlockLangEvent:
			ops = append(ops, Operation{Kind: "code_lang", Value: e.Value})
		case event.HorizontalRuleEvent:
			ops = append(ops, Operation{Kind: "thematic_break"})
		case event.BulletItemEvent:
			kind := "bullet"
			if e.Value != "" {
				kind = "ordered"
			}
			ops = append(ops, Operation{Kind: "list_item", Value: kind})
		case event.BlockquoteStartEvent:
			ops = append(ops, Operation{Kind: "blockquote_start"})
		case event.BlockquoteEndEvent:
			ops = append(ops, Operation{Kind: "blockquote_end"})
		case event.LinkEvent:
			ops = append(ops, Operation{Kind: "link", Value: e.Value, URL: e.URL, Title: e.Title})
		case event.ImageEvent:
			ops = append(ops, Operation{Kind: "image", Value: e.Value, URL: e.URL, Title: e.Title})
		case event.AutolinkURLEvent, event.AutolinkEmailEvent:
			ops = append(ops, Operation{Kind: "link", Value: e.Value, URL: e.URL})
		case event.HTMLBlockStartEvent, event.HTMLBlockEndEvent:
			return Projection{Reason: "HTML block event is intentionally outside terminal semantics"}
		case event.LinkRefDefEvent, event.LinkRefEvent, event.ImageRefEvent:
			return Projection{Reason: "reference-link resolution is intentionally streaming-limited"}
		case event.TableStartEvent, event.TableRowEvent, event.TableEndEvent:
			return Projection{Reason: "GFM table extension is outside CommonMark"}
		default:
			return Projection{Reason: fmt.Sprintf("event %s is not represented by the diagnostic oracle", e.Type)}
		}
	}
	return Projection{Operations: canonicalize(ops), Comparable: true}
}

// Difference describes the earliest semantic mismatch.
type Difference struct {
	Index    int        `json:"index"`
	Expected *Operation `json:"expected,omitempty"`
	Actual   *Operation `json:"actual,omitempty"`
}

// FirstDifference compares semantic operation streams and returns the first
// mismatch. Nil means the projections are equivalent for this oracle.
func FirstDifference(expected, actual []Operation) *Difference {
	limit := len(expected)
	if len(actual) < limit {
		limit = len(actual)
	}
	for i := 0; i < limit; i++ {
		if expected[i] != actual[i] {
			e := expected[i]
			a := actual[i]
			return &Difference{Index: i, Expected: &e, Actual: &a}
		}
	}
	if len(expected) == len(actual) {
		return nil
	}
	d := &Difference{Index: limit}
	if limit < len(expected) {
		e := expected[limit]
		d.Expected = &e
	}
	if limit < len(actual) {
		a := actual[limit]
		d.Actual = &a
	}
	return d
}

type htmlProjectionParser struct {
	input string
	pos   int
	ops   []Operation

	blocks []string
	lists  []string
	links  []linkCapture
	reason string
}

type linkCapture struct {
	url   string
	title string
	text  strings.Builder
}

func (p *htmlProjectionParser) parse() {
	for p.pos < len(p.input) && p.reason == "" {
		if p.input[p.pos] != '<' {
			next := strings.IndexByte(p.input[p.pos:], '<')
			if next < 0 {
				next = len(p.input) - p.pos
			}
			p.emitText(p.input[p.pos : p.pos+next])
			p.pos += next
			continue
		}

		if strings.HasPrefix(p.input[p.pos:], "<!--") {
			end := strings.Index(p.input[p.pos+4:], "-->")
			if end < 0 {
				p.reason = "malformed expected HTML comment"
				return
			}
			p.reason = "raw HTML is outside the semantic adapter"
			return
		}

		start := p.pos
		raw, end, ok := scanTag(p.input, p.pos)
		if !ok {
			p.emitText("<")
			p.pos++
			continue
		}
		if !looksLikeHTMLTag(raw) {
			p.emitText("<")
			p.pos = start + 1
			continue
		}
		p.pos = end
		p.handleTag(raw)
	}

	if p.reason == "" && (len(p.blocks) != 0 || len(p.lists) != 0 || len(p.links) != 0) {
		p.reason = "unbalanced expected HTML structure"
	}
}

func scanTag(input string, start int) (string, int, bool) {
	quote := byte(0)
	for i := start + 1; i < len(input); i++ {
		c := input[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '>' {
			return input[start+1 : i], i + 1, true
		}
	}
	return "", 0, false
}

func (p *htmlProjectionParser) handleTag(raw string) {
	name, closing, selfClosing, attrs, ok := parseTag(raw)
	if !ok {
		p.reason = "malformed expected HTML tag"
		return
	}

	if len(p.links) > 0 && name != "a" {
		p.reason = "nested formatting inside a link is outside the event contract"
		return
	}

	switch name {
	case "p":
		if closing {
			p.closeBlock("p", true)
			return
		}
		p.openBlock("p")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(name[1] - '0')
		if closing {
			p.closeBlock(name, false)
			if p.reason == "" {
				p.ops = append(p.ops, Operation{Kind: "heading_end"}, Operation{Kind: "newline"})
			}
			return
		}
		p.openBlock(name)
		if p.reason == "" {
			p.ops = append(p.ops, Operation{Kind: "heading_start", Level: level})
		}
	case "em":
		p.inlineTag(closing, "em_start", "em_end")
	case "strong":
		p.inlineTag(closing, "strong_start", "strong_end")
	case "del":
		p.inlineTag(closing, "strike_start", "strike_end")
	case "code":
		if p.inBlock("pre") {
			if closing {
				return
			}
			if class := attrs["class"]; class != "" {
				const prefix = "language-"
				if strings.HasPrefix(class, prefix) {
					p.ops = append(p.ops, Operation{Kind: "code_lang", Value: strings.TrimPrefix(class, prefix)})
				}
			}
			return
		}
		p.inlineTag(closing, "code_start", "code_end")
	case "pre":
		if closing {
			p.closeBlock("pre", false)
			if p.reason == "" {
				// The newline after </pre> is HTML serialization whitespace;
				// the code content's own final newline is the parser event.
				p.ops = append(p.ops, Operation{Kind: "code_block_end"})
			}
			return
		}
		p.openBlock("pre")
		if p.reason == "" {
			p.ops = append(p.ops, Operation{Kind: "code_block_start"})
		}
	case "blockquote":
		if closing {
			p.closeBlock("blockquote", false)
			if p.reason == "" {
				p.ops = append(p.ops, Operation{Kind: "blockquote_end"})
			}
			return
		}
		p.openBlock("blockquote")
		if p.reason == "" {
			p.ops = append(p.ops, Operation{Kind: "blockquote_start"})
		}
	case "ul", "ol":
		if closing {
			if len(p.lists) == 0 || p.lists[len(p.lists)-1] != name {
				p.reason = "unbalanced expected list"
				return
			}
			p.lists = p.lists[:len(p.lists)-1]
			return
		}
		if len(p.lists) > 0 {
			p.reason = "nested lists are outside the streaming compatibility subset"
			return
		}
		p.lists = append(p.lists, name)
	case "li":
		if closing {
			p.closeBlock("li", true)
			return
		}
		if len(p.lists) == 0 {
			p.reason = "list item without list"
			return
		}
		p.openBlock("li")
		if p.reason == "" {
			kind := "bullet"
			if p.lists[len(p.lists)-1] == "ol" {
				kind = "ordered"
			}
			p.ops = append(p.ops, Operation{Kind: "list_item", Value: kind})
		}
	case "a":
		if closing {
			if len(p.links) == 0 {
				p.reason = "unbalanced expected link"
				return
			}
			capture := p.links[len(p.links)-1]
			p.links = p.links[:len(p.links)-1]
			p.ops = append(p.ops, Operation{Kind: "link", Value: html.UnescapeString(capture.text.String()), URL: capture.url, Title: capture.title})
			return
		}
		if selfClosing || len(p.links) > 0 {
			p.reason = "nested or self-closing link is outside the event contract"
			return
		}
		p.links = append(p.links, linkCapture{url: html.UnescapeString(attrs["href"]), title: html.UnescapeString(attrs["title"])})
	case "img":
		if closing || !selfClosing {
			p.reason = "malformed expected image"
			return
		}
		p.ops = append(p.ops, Operation{Kind: "image", Value: html.UnescapeString(attrs["alt"]), URL: html.UnescapeString(attrs["src"]), Title: html.UnescapeString(attrs["title"])})
	case "br":
		if closing || !selfClosing {
			p.reason = "malformed expected hard break"
			return
		}
		p.ops = append(p.ops, Operation{Kind: "newline"})
	case "hr":
		if closing || !selfClosing {
			p.reason = "malformed expected thematic break"
			return
		}
		p.ops = append(p.ops, Operation{Kind: "thematic_break"}, Operation{Kind: "newline"})
	default:
		p.reason = "raw or unsupported HTML tag <" + name + ">"
	}
}

func (p *htmlProjectionParser) openBlock(name string) {
	if len(p.blocks) > 0 {
		parent := p.blocks[len(p.blocks)-1]
		allowed := (parent == "blockquote" && name == "p") ||
			((parent == "ul" || parent == "ol") && name == "li") ||
			(parent == "li" && name == "p") ||
			(parent == "pre" && name == "code")
		if !allowed {
			p.reason = "unsupported nested block structure"
			return
		}
	}
	p.blocks = append(p.blocks, name)
}

func (p *htmlProjectionParser) closeBlock(name string, newline bool) {
	if len(p.blocks) == 0 || p.blocks[len(p.blocks)-1] != name {
		p.reason = "unbalanced expected block <" + name + ">"
		return
	}
	p.blocks = p.blocks[:len(p.blocks)-1]
	if newline {
		p.ops = append(p.ops, Operation{Kind: "newline"})
	}
}

func (p *htmlProjectionParser) inlineTag(closing bool, start, end string) {
	if closing {
		p.ops = append(p.ops, Operation{Kind: end})
		return
	}
	p.ops = append(p.ops, Operation{Kind: start})
}

func (p *htmlProjectionParser) emitText(raw string) {
	if raw == "" {
		return
	}
	decoded := html.UnescapeString(raw)
	if len(p.links) > 0 {
		p.links[len(p.links)-1].text.WriteString(decoded)
		return
	}
	if strings.TrimSpace(decoded) == "" && !p.textBlockOpen() {
		return
	}

	for len(decoded) > 0 {
		idx := strings.IndexByte(decoded, '\n')
		if idx < 0 {
			if decoded != "" {
				p.ops = append(p.ops, Operation{Kind: "text", Value: decoded})
			}
			return
		}
		if idx > 0 {
			p.ops = append(p.ops, Operation{Kind: "text", Value: decoded[:idx]})
		}
		p.ops = append(p.ops, Operation{Kind: "newline"})
		decoded = decoded[idx+1:]
	}
}

func (p *htmlProjectionParser) inBlock(name string) bool {
	for i := len(p.blocks) - 1; i >= 0; i-- {
		if p.blocks[i] == name {
			return true
		}
	}
	return false
}

func (p *htmlProjectionParser) textBlockOpen() bool {
	if len(p.blocks) == 0 {
		return false
	}
	switch p.blocks[len(p.blocks)-1] {
	case "p", "h1", "h2", "h3", "h4", "h5", "h6", "pre", "li":
		return true
	default:
		return false
	}
}

func parseTag(raw string) (name string, closing, selfClosing bool, attrs map[string]string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "!") || strings.HasPrefix(raw, "?") {
		return "", false, false, nil, false
	}
	if raw[0] == '/' {
		closing = true
		raw = strings.TrimSpace(raw[1:])
	}
	if strings.HasSuffix(raw, "/") {
		selfClosing = true
		raw = strings.TrimSpace(strings.TrimSuffix(raw, "/"))
	}
	if raw == "" {
		return "", false, false, nil, false
	}

	i := 0
	for i < len(raw) && (raw[i] == '-' || raw[i] == ':' || raw[i] == '_' || unicode.IsLetter(rune(raw[i])) || unicode.IsDigit(rune(raw[i]))) {
		i++
	}
	if i == 0 {
		return "", false, false, nil, false
	}
	name = strings.ToLower(raw[:i])
	attrs = make(map[string]string)
	for i < len(raw) {
		for i < len(raw) && unicode.IsSpace(rune(raw[i])) {
			i++
		}
		if i == len(raw) {
			break
		}
		start := i
		for i < len(raw) && (raw[i] == '-' || raw[i] == ':' || raw[i] == '_' || unicode.IsLetter(rune(raw[i])) || unicode.IsDigit(rune(raw[i]))) {
			i++
		}
		if start == i {
			return "", false, false, nil, false
		}
		key := strings.ToLower(raw[start:i])
		for i < len(raw) && unicode.IsSpace(rune(raw[i])) {
			i++
		}
		if i == len(raw) || raw[i] != '=' {
			return "", false, false, nil, false
		}
		i++
		for i < len(raw) && unicode.IsSpace(rune(raw[i])) {
			i++
		}
		if i == len(raw) || (raw[i] != '\'' && raw[i] != '"') {
			return "", false, false, nil, false
		}
		quote := raw[i]
		i++
		valueStart := i
		for i < len(raw) && raw[i] != quote {
			i++
		}
		if i == len(raw) {
			return "", false, false, nil, false
		}
		attrs[key] = raw[valueStart:i]
		i++
	}
	return name, closing, selfClosing, attrs, true
}

func looksLikeHTMLTag(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if raw[0] == '!' || raw[0] == '?' {
		return true
	}
	if raw[0] == '/' {
		raw = strings.TrimSpace(raw[1:])
	}
	if raw == "" || !isHTMLNameStart(raw[0]) {
		return false
	}
	for i := 1; i < len(raw); i++ {
		c := raw[i]
		if isHTMLNameStart(c) || unicode.IsDigit(rune(c)) || c == '-' || c == '_' {
			continue
		}
		return unicode.IsSpace(rune(c)) || c == '/'
	}
	return true
}

func canonicalize(input []Operation) []Operation {
	output := make([]Operation, 0, len(input))
	codeOpening := false
	for _, op := range input {
		if op.Kind == "text" && op.Value == "" {
			continue
		}
		if op.Kind == "blockquote_end" && len(output) > 0 && output[len(output)-1].Kind != "newline" {
			// HTML closes the quote paragraph even when the terminal event
			// stream ends the style directly at EOF. Represent that logical
			// paragraph boundary without changing terminal presentation.
			output = append(output, Operation{Kind: "newline"})
		}
		if op.Kind == "newline" {
			if len(output) > 0 && output[len(output)-1].Kind == "blockquote_end" {
				// The terminal writer closes quote styling before it writes the
				// separating newline. HTML closes the paragraph first. They are
				// the same block boundary in the semantic projection.
				if len(output) > 1 && output[len(output)-2].Kind == "newline" {
					continue
				}
				output[len(output)-1] = op
				output = append(output, Operation{Kind: "blockquote_end"})
				continue
			}
			if codeOpening {
				codeOpening = false
				continue
			}
			if len(output) > 0 && output[len(output)-1].Kind == "code_block_end" {
				// Closing-fence line endings are parser/terminal framing, not
				// content in the CommonMark <pre><code> semantic projection.
				continue
			}
			if len(output) > 0 && output[len(output)-1].Kind == "newline" {
				continue
			}
			output = append(output, op)
			continue
		}
		if op.Kind == "text" && len(output) > 0 && output[len(output)-1].Kind == "text" {
			output[len(output)-1].Value += op.Value
			codeOpening = false
			continue
		}
		output = append(output, op)
		codeOpening = op.Kind == "code_block_start" || op.Kind == "code_lang"
	}
	return output
}

func containsRawHTML(markdown string) bool {
	for i := 0; i < len(markdown); i++ {
		if markdown[i] != '<' || i+1 >= len(markdown) {
			continue
		}
		next := markdown[i+1]
		if next == '!' || next == '?' {
			return true
		}
		if !isHTMLNameStart(next) {
			continue
		}
		j := i + 2
		for j < len(markdown) && (isHTMLNameStart(markdown[j]) || unicode.IsDigit(rune(markdown[j])) || markdown[j] == '-' || markdown[j] == ':') {
			j++
		}
		if j < len(markdown) && (markdown[j] == '>' || markdown[j] == '/' || unicode.IsSpace(rune(markdown[j]))) {
			return true
		}
	}
	return false
}

func isHTMLNameStart(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}
