package parser

// lineContext owns context whose meaning ends at a line boundary.
type lineContext struct {
	lineStart     bool
	prevChar      byte
	contentIndent int
}
