package parser

import (
	"strings"

	"github.com/cjccjj/mdflow/pkg/markdown/tokenizer"
)

func (p *Parser) enterState(state State) {
	p.state = state
}

// tryThematicBreak checks for a horizontal rule using all three marker types.
func (p *Parser) tryThematicBreak() ([]Event, bool) {
	for _, marker := range []tokenizer.TokenType{tokenizer.DashToken, tokenizer.StarToken, tokenizer.UnderscoreToken} {
		matched, waiting := p.checkHorizontalRule(marker)
		if matched {
			return []Event{{Type: HorizontalRuleEvent}}, true
		}
		if waiting {
			return nil, true
		}
	}
	return nil, false
}

// tryOrderedList checks for an ordered list prefix (like "1. " or "2) ").
func (p *Parser) tryOrderedList() ([]Event, bool) {
	if p.buf[0].Type != tokenizer.TextToken {
		return nil, false
	}
	fullValue := p.buf[0].Value
	prefix, ok := orderedListPrefix(fullValue)
	var rest string
	if !ok {
		if isDigitsOnly(fullValue) && len(p.buf) > 1 && p.buf[1].Type == tokenizer.RightParenToken {
			p.consume(2)
			prefix = fullValue + ")"
			if len(p.buf) > 0 && p.buf[0].Type == tokenizer.TextToken {
				rest = p.buf[0].Value
				p.consume(1)
				if strings.HasPrefix(rest, " ") {
					prefix += " "
					rest = rest[1:]
				}
			}
		} else {
			return nil, false
		}
	} else {
		p.consume(1)
		rest = fullValue[len(prefix):]
	}
	p.lineStart = false
	events := []Event{{Type: BulletItemEvent, Value: prefix}}
	p.listContentIndent = p.lineStartIndent + len(prefix)
	if strings.HasPrefix(rest, "    ") {
		p.enterState(IndentedCodeBlockState)
		codeContent := rest[4:]
		events = append(events, Event{Type: CodeBlockStartEvent})
		if codeContent != "" {
			events = append(events, Event{Type: TextEvent, Value: codeContent})
		}
		return events, true
	}
	if rest != "" {
		events = append(events, Event{Type: TextEvent, Value: rest})
	}
	return events, true
}

// tryIndentedCodeOrList handles 4+ spaces of indentation.
func (p *Parser) tryIndentedCodeOrList() ([]Event, bool) {
	satisfied, consumeCount, remaining := p.peekEquivIndent()
	if !satisfied {
		return nil, false
	}

	// If we're inside a list item (listContentIndent > 0), check whether the
	// indentation is a continuation of the list rather than a code block.
	// CommonMark: a continuation line needs at least listContentIndent spaces;
	// if the indent beyond that is 4+, it's an indented code block.
	if p.listContentIndent > 0 {
		// Compute the total indent column from leading whitespace only.
		col := 0
		for _, tok := range p.buf[:consumeCount] {
			if tok.Type == tokenizer.TabToken {
				col = ((col + 4) / 4) * 4
			} else if tok.Type == tokenizer.TextToken {
				for _, r := range tok.Value {
					if r != ' ' {
						break
					}
					col++
				}
			}
		}
		if col >= p.listContentIndent && col < p.listContentIndent+4 {
			// List continuation: strip the indent and emit the content.
			p.consume(consumeCount)
			p.lineStart = false
			p.lineStartIndent = 0
			if remaining != "" {
				// remaining still has (col - 4) leading spaces from the
				// peekEquivIndent threshold. Strip the full list indent.
				extra := col - 4
				if extra > 0 && len(remaining) >= extra {
					remaining = remaining[extra:]
				}
				return []Event{{Type: TextEvent, Value: remaining}}, true
			}
			return nil, true
		}
	}

	if isListStartAfterIndent(p.buf[consumeCount:], remaining) {
		p.consume(consumeCount)
		p.enterState(IndentedCodeBlockState)
		p.lineStart = false
		p.listContentIndent = 0
		events := []Event{{Type: CodeBlockStartEvent}}
		if remaining != "" {
			events = append(events, Event{Type: TextEvent, Value: remaining})
		}
		return events, true
	}
	p.consume(consumeCount)
	p.enterState(IndentedCodeBlockState)
	p.lineStart = false
	p.listContentIndent = 0
	events := []Event{{Type: CodeBlockStartEvent}}
	if remaining != "" {
		events = append(events, Event{Type: TextEvent, Value: remaining})
	}
	return events, true
}

// trySetextCandidate buffers a text line as a potential setext heading.
func (p *Parser) trySetextCandidate() ([]Event, bool) {
	if p.buf[0].Type != tokenizer.TextToken {
		return nil, false
	}
	if !p.hasNewline() {
		return nil, false
	}
	p.enterState(SetextPendingState)
	return nil, true
}

// tryATXHeading handles all ATX heading cases.
func (p *Parser) tryATXHeading() ([]Event, bool) {
	first := p.buf[0]

	if first.Type == tokenizer.HashToken {
		hashCount := 0
		for hashCount < len(p.buf) && p.buf[hashCount].Type == tokenizer.HashToken {
			hashCount++
		}
		if hashCount > 6 {
			p.consume(hashCount)
			p.lineStart = false
			events := make([]Event, hashCount)
			for i := 0; i < hashCount; i++ {
				events[i] = Event{Type: TextEvent, Value: "#"}
			}
			return events, true
		}
		events := p.tryHeader()
		return events, events != nil
	}

	if first.Type == tokenizer.TextToken && len(p.buf) > 1 {
		if sp := leadingSpaceCount(first.Value); sp >= 1 && sp <= 3 && p.buf[1].Type == tokenizer.HashToken {
			p.consume(1)
			events := p.tryHeader()
			return events, events != nil
		}
	}

	return nil, false
}

func (p *Parser) bufferHasPattern() bool {
	if len(p.buf) == 0 {
		return false
	}
	t := p.buf[0].Type
	if p.lineStart {
		switch t {
		case tokenizer.HashToken, tokenizer.GreaterToken, tokenizer.DashToken,
			tokenizer.StarToken, tokenizer.UnderscoreToken, tokenizer.PipeToken,
			tokenizer.TabToken, tokenizer.LeftBracketToken:
			return true
		case tokenizer.TextToken:
			return hasLineStartTextPattern(p.buf[0].Value)
		}
	}
	switch t {
	case tokenizer.StarToken, tokenizer.BacktickToken, tokenizer.TildeToken,
		tokenizer.UnderscoreToken, tokenizer.LeftBracketToken,
		tokenizer.BackslashToken, tokenizer.AmpersandToken,
		tokenizer.HashToken, tokenizer.DashToken, tokenizer.GreaterToken,
		tokenizer.PipeToken:
		return true
	case tokenizer.TextToken:
		if p.lineStart {
			return hasLineStartTextPattern(p.buf[0].Value)
		}
	}
	return false
}

func hasLineStartTextPattern(v string) bool {
	if len(v) == 0 {
		return false
	}
	if v[0] == '<' {
		return true
	}
	if v[0] == ' ' {
		// A line indented by up to three spaces still needs the deferred
		// line-start pass: it may be a list marker, heading, or ordinary
		// paragraph whose indentation CommonMark ignores. Without this stop,
		// emitTextOrSpecial can consume it in the same chunk as a preceding
		// blank line and make parsing chunk-dependent.
		if leadingSpaceCount(v) <= 3 {
			return true
		}
	}
	if _, ok := orderedListPrefix(v); ok {
		return true
	}
	if isDigitsOnly(v) {
		return true
	}
	return false
}
