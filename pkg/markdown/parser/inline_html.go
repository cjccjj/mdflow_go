package parser

import (
	"strings"

	"github.com/cjccjj/mdflow/pkg/markdown/tokenizer"
)

const (
	htmlTagOpen = iota + 1
	htmlTagClose
	htmlTagComment
	htmlTagPI
	htmlTagDecl
	htmlTagCDATA
	htmlTagIncomplete
)

func (p *Parser) tryInlineHTML() ([]Event, bool) {
	if len(p.buf) == 0 {
		return nil, false
	}

	first := p.buf[0]
	if first.Type != tokenizer.TextToken {
		return nil, false
	}

	ltIdx := strings.Index(first.Value, "<")
	if ltIdx == -1 {
		return nil, false
	}

	afterLT := first.Value[ltIdx+1:]

	tagType, valid := classifyHTMLStart(afterLT, p.buf)
	if !valid {
		return nil, false
	}
	if tagType == htmlTagIncomplete {
		if p.eof {
			return nil, false
		}
		return nil, true
	}

	if tagType == htmlTagOpen && bufferLooksLikeAutolink(afterLT, p.buf) {
		return nil, false
	}

	endInfo := findHTMLTagEnd(p.buf, tagType, ltIdx)
	if endInfo < 0 {
		if p.eof {
			return nil, false
		}

		var events []Event
		if ltIdx > 0 {
			events = append(events, Event{Type: TextEvent, Value: first.Value[:ltIdx]})
		}
		return events, true
	}

	var events []Event
	if ltIdx > 0 {
		events = append(events, Event{Type: TextEvent, Value: first.Value[:ltIdx]})
	}

	p.consume(endInfo)
	p.lineStart = false
	return events, true
}

func classifyHTMLStart(afterLT string, buf []tokenizer.Token) (tagType int, valid bool) {
	if len(afterLT) == 0 {
		return classifyFromNextToken(buf)
	}

	switch afterLT[0] {
	case '/':
		if len(afterLT) >= 2 && isASCIILetter(afterLT[1]) {
			return htmlTagClose, true
		}
		if len(afterLT) == 1 && len(buf) > 1 && buf[1].Type == tokenizer.TextToken {
			if len(buf[1].Value) > 0 && isASCIILetter(buf[1].Value[0]) {
				return htmlTagClose, true
			}
		}
		return 0, false
	case '!':
		return classifyBangStart(afterLT, buf)
	case '?':
		return htmlTagPI, true
	default:
		if isASCIILetter(afterLT[0]) {
			return htmlTagOpen, true
		}
		return 0, false
	}
}

func classifyFromNextToken(buf []tokenizer.Token) (int, bool) {
	if len(buf) <= 1 {
		return htmlTagIncomplete, true
	}
	next := buf[1]
	if next.Type == tokenizer.GreaterToken {
		return 0, false
	}
	if next.Type == tokenizer.TextToken && len(next.Value) > 0 {
		return classifyHTMLStart(next.Value, buf[1:])
	}
	return htmlTagIncomplete, true
}

func classifyBangStart(afterLT string, buf []tokenizer.Token) (int, bool) {
	if strings.HasPrefix(afterLT, "!--") {
		return htmlTagComment, true
	}

	if strings.HasPrefix(afterLT, "![CDATA[") {
		return htmlTagCDATA, true
	}

	if len(afterLT) >= 2 && isASCIILetter(afterLT[1]) {
		return htmlTagDecl, true
	}

	nextIdx := 1
	content := afterLT

	for nextIdx < len(buf) && len(content) < 10 {
		tok := buf[nextIdx]
		if tok.Type == tokenizer.NewlineToken || tok.Type == tokenizer.GreaterToken {
			break
		}
		content += tok.Value
		nextIdx++
	}

	if strings.HasPrefix(content, "!--") {
		return htmlTagComment, true
	}
	if strings.HasPrefix(content, "![CDATA[") {
		return htmlTagCDATA, true
	}
	if len(content) >= 2 && isASCIILetter(content[1]) {
		return htmlTagDecl, true
	}

	if strings.HasPrefix(content, "!") || strings.HasPrefix(content, "!-") {
		return htmlTagIncomplete, true
	}
	if afterLT == "!" && len(buf) > 1 && buf[1].Type == tokenizer.DashToken {
		return htmlTagIncomplete, true
	}

	return 0, false
}

func findHTMLTagEnd(buf []tokenizer.Token, tagType int, ltIdx int) int {
	switch tagType {
	case htmlTagOpen:
		return findOpenTagEnd(buf, ltIdx)
	case htmlTagClose:
		return findOpenTagEnd(buf, ltIdx)
	case htmlTagComment:
		return findCommentEnd(buf)
	case htmlTagPI:
		return findPIEnd(buf)
	case htmlTagDecl:
		return findDeclEnd(buf)
	case htmlTagCDATA:
		return findCDATAEnd(buf)
	}
	return -1
}

func findOpenTagEnd(buf []tokenizer.Token, ltIdx int) int {
	inSQ, inDQ := false, false

	firstVal := buf[0].Value
	for j := ltIdx + 1; j < len(firstVal); j++ {
		c := firstVal[j]
		if c == '\'' && !inDQ {
			inSQ = !inSQ
		} else if c == '"' && !inSQ {
			inDQ = !inDQ
		}
	}

	for i := 1; i < len(buf); i++ {
		tok := buf[i]
		if tok.Type == tokenizer.NewlineToken {
			return -2
		}

		if tok.Type == tokenizer.GreaterToken && !inSQ && !inDQ {
			return i + 1
		}

		if tok.Type == tokenizer.TextToken {
			for _, c := range tok.Value {
				if c == '\'' && !inDQ {
					inSQ = !inSQ
				} else if c == '"' && !inSQ {
					inDQ = !inDQ
				}
			}
		}
	}
	return -1
}

func findCommentEnd(buf []tokenizer.Token) int {
	for i, tok := range buf {
		if tok.Type == tokenizer.NewlineToken {
			return -2
		}

		if tok.Type == tokenizer.GreaterToken {
			if i >= 1 && hasPrecedingDashDash(buf, i) {
				return i + 1
			}
		}
	}
	return -1
}

func hasPrecedingDashDash(buf []tokenizer.Token, gtIdx int) bool {
	prev := buf[gtIdx-1]
	if prev.Type == tokenizer.TextToken && strings.HasSuffix(prev.Value, "--") {
		return true
	}
	if prev.Type == tokenizer.DashToken {
		if gtIdx >= 2 && buf[gtIdx-2].Type == tokenizer.DashToken {
			return true
		}
		if gtIdx >= 2 && buf[gtIdx-2].Type == tokenizer.TextToken && strings.HasSuffix(buf[gtIdx-2].Value, "-") {
			return true
		}
	}
	return false
}

func findPIEnd(buf []tokenizer.Token) int {
	for i, tok := range buf {
		if tok.Type == tokenizer.NewlineToken {
			return -2
		}

		if tok.Type == tokenizer.GreaterToken {
			if i >= 1 && hasPrecedingQuestionMark(buf, i) {
				return i + 1
			}
		}
	}
	return -1
}

func hasPrecedingQuestionMark(buf []tokenizer.Token, gtIdx int) bool {
	prev := buf[gtIdx-1]
	if prev.Type == tokenizer.TextToken && strings.HasSuffix(prev.Value, "?") {
		return true
	}
	return false
}

func findDeclEnd(buf []tokenizer.Token) int {
	for i, tok := range buf {
		if tok.Type == tokenizer.NewlineToken {
			return -2
		}

		if tok.Type == tokenizer.GreaterToken {
			if i == 0 {
				return -2
			}
			return i + 1
		}
	}
	return -1
}

func findCDATAEnd(buf []tokenizer.Token) int {
	for i, tok := range buf {
		if tok.Type == tokenizer.NewlineToken {
			return -2
		}

		if tok.Type == tokenizer.GreaterToken {
			if i >= 1 && hasPrecedingBracketBracket(buf, i) {
				return i + 1
			}
		}
	}
	return -1
}

func hasPrecedingBracketBracket(buf []tokenizer.Token, gtIdx int) bool {
	prev := buf[gtIdx-1]
	if prev.Type == tokenizer.TextToken && strings.HasSuffix(prev.Value, "]]") {
		return true
	}
	if prev.Type == tokenizer.RightBracketToken {
		if gtIdx >= 2 && buf[gtIdx-2].Type == tokenizer.RightBracketToken {
			return true
		}
		if gtIdx >= 2 && buf[gtIdx-2].Type == tokenizer.TextToken && strings.HasSuffix(buf[gtIdx-2].Value, "]") {
			return true
		}
	}
	return false
}

func bufferLooksLikeAutolink(afterLT string, buf []tokenizer.Token) bool {
	if looksLikeAutolinkTagName(afterLT) {
		return true
	}
	if len(afterLT) == 0 && len(buf) > 1 && buf[1].Type == tokenizer.TextToken {
		return looksLikeAutolinkTagName(buf[1].Value)
	}
	return false
}

func looksLikeAutolinkTagName(s string) bool {
	for _, c := range s {
		if c == ' ' || c == '\t' {
			break
		}
		if c == '@' || c == ':' {
			return true
		}
	}
	return false
}
