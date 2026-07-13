// Package event defines the semantic events exchanged by the Markdown parser
// and renderers. It intentionally has no dependencies on parsing or output
// packages so consumers can depend on the contract without importing either.
package event

// Type identifies a semantic Markdown event.
type Type int

const (
	TextEvent Type = iota
	HeaderStartEvent
	HeaderEndEvent
	BoldStartEvent
	BoldEndEvent
	ItalicStartEvent
	ItalicEndEvent
	StrikethroughStartEvent
	StrikethroughEndEvent
	InlineCodeStartEvent
	InlineCodeEndEvent
	CodeBlockStartEvent
	CodeBlockEndEvent
	CodeBlockLangEvent
	HorizontalRuleEvent
	BulletItemEvent
	NewlineEvent
	TableStartEvent
	TableRowEvent
	TableEndEvent
	BlockquoteStartEvent
	BlockquoteEndEvent
	LinkEvent
	ImageEvent
	HTMLBlockStartEvent
	HTMLBlockEndEvent
	LinkRefDefEvent
	LinkRefEvent
	ImageRefEvent
	AutolinkURLEvent
	AutolinkEmailEvent
)

// Event is one semantic unit emitted by the parser.
type Event struct {
	Type   Type
	Value  string
	Level  int
	Cells  []string
	Widths []int
	Aligns []int
	URL    string
	Title  string
}

func (t Type) String() string {
	names := [...]string{
		"Text", "HeaderStart", "HeaderEnd", "BoldStart", "BoldEnd", "ItalicStart", "ItalicEnd",
		"StrikethroughStart", "StrikethroughEnd", "InlineCodeStart", "InlineCodeEnd", "CodeBlockStart",
		"CodeBlockEnd", "CodeBlockLang", "HorizontalRule", "BulletItem", "Newline", "TableStart",
		"TableRow", "TableEnd", "BlockquoteStart", "BlockquoteEnd", "Link", "Image", "HTMLBlockStart",
		"HTMLBlockEnd", "LinkRefDef", "LinkRef", "ImageRef", "AutolinkURL", "AutolinkEmail",
	}
	if int(t) < 0 || int(t) >= len(names) {
		return "Unknown"
	}
	return names[t]
}
