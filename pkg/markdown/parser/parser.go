package parser

import (
	"regexp"
	"strings"

	"github.com/cjccjj/mdflow/pkg/markdown/tokenizer"
)

type Parser struct {
	state State
	tokenBuffer
	lineContext
	blockParser      *blockParser
	linkRefDefParser *linkRefDefParser
	setextParser     *setextParser
	htmlBlockParser  *htmlBlockParser

	linkParser     *linkParser
	emphasisParser *emphasisParser
	tableParser    *tableParser
	trace          *traceRecorder
}

func New() *Parser {
	p := &Parser{state: NormalState, lineContext: lineContext{lineStart: true}}
	p.linkParser = newLinkParser(p)
	p.emphasisParser = newEmphasisParser(p)
	p.tableParser = newTableParser(p)
	p.blockParser = newBlockParser(p)
	p.linkRefDefParser = newLinkRefDefParser(p)
	p.setextParser = newSetextParser(p)
	p.htmlBlockParser = newHTMLBlockParser(p)
	return p
}

func (p *Parser) Reset() {
	p.state = NormalState
	p.tokenBuffer = tokenBuffer{}
	p.lineContext = lineContext{lineStart: true}
	p.blockParser.reset()
	p.linkParser.reset()
	p.emphasisParser.reset()
	p.tableParser.reset()
	p.linkRefDefParser.reset()
	p.setextParser.reset()
	p.htmlBlockParser.reset()
}

func (p *Parser) Parse(tokens []tokenizer.Token) (events []Event) {
	if len(tokens) == 0 {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			events = p.safeFlush()
		}
	}()

	p.appendTokens(tokens)
	return p.process()
}

func (p *Parser) process() []Event {
	var events []Event

	for p.hasBufferedTokens() {
		prevLen := p.bufferedLen()
		prevState := p.state
		p.trace.begin(p)
		var iteration []Event

		switch p.state {

		case NormalState:
			p.trace.branch("state.normal")
			iteration = p.processNormal()

		case HeaderState:
			p.trace.branch("state.header")
			iteration = p.processHeader()

		case InlineCodeState:
			p.trace.branch("state.inline_code")
			iteration = p.processInlineCode()

		case CodeBlockState:
			p.trace.branch("state.code_block")
			iteration = p.processCodeBlock()

		case IndentedCodeBlockState:
			p.trace.branch("state.indented_code_block")
			iteration = p.processIndentedCodeBlock()

		case BlockquoteState:
			p.trace.branch("state.blockquote")
			iteration = p.processBlockquote()

		case TablePendingState:
			p.trace.branch("state.table_pending")
			iteration = p.tableParser.processTablePending()

		case TableBodyState:
			p.trace.branch("state.table_body")
			iteration = p.tableParser.processTableBody()

		case SetextPendingState:
			p.trace.branch("state.setext_pending")
			iteration = p.processSetextPending()

		case LinkTextState:
			p.trace.branch("state.link_text")
			iteration = p.linkParser.processLinkText()

		case LinkURLState:
			p.trace.branch("state.link_url")
			iteration = p.linkParser.processLinkURL()

		case HTMLBlockState:
			p.trace.branch("state.html_block")
			iteration = p.processHTMLBlock()

		case LinkRefDefState:
			p.trace.branch("state.link_ref_definition")
			iteration = p.linkRefDefParser.processLinkRefDef()
		}
		events = append(events, iteration...)

		if p.bufferedLen() == prevLen && p.state == prevState {
			p.trace.finish(p, iteration, "wait")
			break
		}
		p.trace.finish(p, iteration, "progress")
	}

	return events
}

func (p *Parser) processNormal() []Event {
	if len(p.buf) == 0 {
		return nil
	}
	if p.lineStart && p.consumeOptionalLineStartIndent() {
		p.trace.branch("line_start.optional_indent")
		return nil
	}

	first := p.buf[0]

	if events, handled := p.processEscapeOrEntity(first); handled {
		if first.Type == tokenizer.BackslashToken {
			p.trace.branch("normal.escape")
		} else {
			p.trace.branch("normal.entity")
		}
		return events
	}

	if p.lineStart {
		if events, handled := p.processLineStartBlock(first); handled {
			return events
		}
	}

	if events, handled := p.processInlineStart(first); handled {
		return events
	}

	if events, handled := p.tryImage(); handled {
		p.trace.branch("normal.image")
		return events
	}

	if events, handled := p.tryAutolink(); handled {
		p.trace.branch("normal.autolink")
		return events
	}

	if events, handled := p.tryInlineHTML(); handled {
		p.trace.branch("normal.inline_html")
		return events
	}

	if p.lineStart {
		if events, handled := p.processDeferredLineStart(first); handled {
			return events
		}
	}

	p.trace.branch("normal.text")
	return p.emitTextOrSpecial()
}

// consumeOptionalLineStartIndent removes up to three leading spaces once the
// next token makes their role clear. It waits for that token when a write ends
// inside the indentation, keeping 1–3-space indentation chunk-independent.
// Four or more spaces remain for the indented-code recognizer.
func (p *Parser) consumeOptionalLineStartIndent() bool {
	spaces := 0
	for i, tok := range p.buf {
		if tok.Type != tokenizer.TextToken {
			if spaces == 0 || spaces > 3 {
				return false
			}
			p.consume(i)
			return true
		}

		leading := leadingSpaceCount(tok.Value)
		spaces += leading
		if spaces > 3 {
			return false
		}
		if leading == len(tok.Value) {
			continue
		}
		if spaces == 0 {
			return false
		}
		p.consume(i)
		p.buf[0].Value = tok.Value[leading:]
		return true
	}

	// The input ends in one to three spaces. Wait for the next token unless
	// EOF has made those spaces ordinary literal text.
	return spaces > 0 && spaces <= 3 && !p.eof
}

func (p *Parser) processEscapeOrEntity(first tokenizer.Token) ([]Event, bool) {
	if first.Type == tokenizer.BackslashToken {
		return p.handleBackslash(), true
	}
	if first.Type == tokenizer.AmpersandToken {
		return p.handleEntity(), true
	}
	return nil, false
}

func (p *Parser) processLineStartBlock(first tokenizer.Token) ([]Event, bool) {
	if events, handled := p.tryFencedCodeBlock(); handled {
		p.trace.branch("line_start.fenced_code")
		return events, true
	}

	if events, handled := p.tryThematicBreak(); handled {
		p.trace.branch("line_start.thematic_break")
		return events, true
	}

	if events, handled := p.tryATXHeading(); handled {
		p.trace.branch("line_start.atx_heading")
		return events, true
	}

	if first.Type == tokenizer.GreaterToken {
		p.trace.branch("line_start.blockquote")
		return p.tryBlockquote(), true
	}

	if first.Type == tokenizer.DashToken {
		p.trace.branch("line_start.bullet_dash")
		return p.tryBullet(), true
	}

	if first.Type == tokenizer.StarToken {
		if events := p.emphasisParser.tryBulletOrBold(); events != nil {
			p.trace.branch("line_start.bullet_or_emphasis_star")
			return events, true
		}
		return nil, false
	}

	if first.Type == tokenizer.TextToken {
		if events, handled := p.tryOrderedList(); handled {
			p.trace.branch("line_start.ordered_list")
			return events, true
		}
	}

	if events, handled := p.tryHTMLBlock(); handled {
		p.trace.branch("line_start.html_block")
		return events, true
	}

	if events, handled := p.linkRefDefParser.tryLinkRefDef(); handled {
		p.trace.branch("line_start.link_ref_definition")
		return events, true
	}

	return nil, false
}

func (p *Parser) processInlineStart(first tokenizer.Token) ([]Event, bool) {
	if first.Type == tokenizer.BacktickToken {
		p.trace.branch("inline.backtick")
		return p.processBacktickStart()
	}

	if first.Type == tokenizer.LeftBracketToken && p.prevChar != '!' {
		p.trace.branch("inline.link")
		p.consume(1)
		p.lineStart = false
		p.linkParser.startLinkText()
		return nil, true
	}

	if first.Type == tokenizer.TildeToken {
		p.trace.branch("inline.tilde")
		return p.emphasisParser.tryTilde()
	}

	if first.Type == tokenizer.StarToken {
		p.trace.branch("inline.star")
		return p.emphasisParser.tryStar()
	}

	if first.Type == tokenizer.UnderscoreToken {
		p.trace.branch("inline.underscore")
		return p.emphasisParser.tryUnderscore()
	}

	return nil, false
}

func (p *Parser) processBacktickStart() ([]Event, bool) {
	if p.lineStart {
		matched, waiting := p.checkConsecutive(tokenizer.BacktickToken, 3)
		if matched {
			n := p.countConsecutive(tokenizer.BacktickToken)
			if n == len(p.buf) && !p.eof {
				return nil, true
			}

			hasBacktickInInfo := false
			for i := n; i < len(p.buf); i++ {
				if p.buf[i].Type == tokenizer.NewlineToken {
					break
				}
				if p.buf[i].Type == tokenizer.BacktickToken {
					hasBacktickInInfo = true
					break
				}
			}
			if hasBacktickInInfo {
				p.consume(n)
				p.state = InlineCodeState
				p.blockParser.fenceLen = n
				return []Event{{Type: InlineCodeStartEvent}}, true
			}

			p.consume(n)
			p.state = CodeBlockState
			p.blockParser.fenceLen = n
			p.blockParser.fenceChar = tokenizer.BacktickToken
			p.blockParser.codeBlockFirst = true
			return []Event{{Type: CodeBlockStartEvent}}, true
		}
		if waiting {
			return nil, true
		}
	}
	n := p.countConsecutive(tokenizer.BacktickToken)
	if n == len(p.buf) && !p.eof {
		return nil, true
	}
	if n == 1 && !hasMatchingCloser(p.buf[n:], tokenizer.BacktickToken, n, false) {
		p.consume(n)
		p.lineStart = false
		var events []Event
		for i := 0; i < n; i++ {
			events = append(events, Event{Type: TextEvent, Value: "`"})
		}
		return events, true
	}
	p.consume(n)
	p.state = InlineCodeState
	p.blockParser.fenceLen = n
	return []Event{{Type: InlineCodeStartEvent}}, true
}

// processTildeStart removed (dead code). Tilde emphasis is handled by emphasisParser.tryTilde().

func (p *Parser) processDeferredLineStart(first tokenizer.Token) ([]Event, bool) {
	if first.Type == tokenizer.PipeToken {
		p.trace.branch("deferred.table")
		return p.tableParser.tryTableHeader(), true
	}

	if events, handled := p.tryIndentedCodeOrList(); handled {
		p.trace.branch("deferred.indented_code")
		return events, true
	}

	if events, handled := p.trySetextCandidate(); handled {
		p.trace.branch("deferred.setext")
		return events, true
	}

	return nil, false
}

func (p *Parser) handleIndentedList() []Event {
	if len(p.buf) == 0 {
		return nil
	}
	first := p.buf[0]
	switch first.Type {
	case tokenizer.DashToken:
		return p.tryBullet()
	case tokenizer.StarToken:
		return p.emphasisParser.tryBulletOrBold()
	case tokenizer.TextToken:
		if prefix, ok := orderedListPrefix(first.Value); ok {
			p.consume(1)
			p.lineStart = false
			events := []Event{{Type: BulletItemEvent, Value: prefix}}
			rest := first.Value[len(prefix):]
			if rest != "" {
				events = append(events, Event{Type: TextEvent, Value: rest})
			}
			return events
		}
	}
	return nil
}

func (p *Parser) handleIndentedListRemaining(remaining string) []Event {
	if prefix, ok := orderedListPrefix(remaining); ok {
		p.lineStart = false
		events := []Event{{Type: BulletItemEvent, Value: prefix}}
		rest := remaining[len(prefix):]
		if rest != "" {
			events = append(events, Event{Type: TextEvent, Value: rest})
		}
		return events
	}

	if isDigitsOnly(remaining) && len(p.buf) > 0 {
		next := p.buf[0]
		if next.Type == tokenizer.RightParenToken || next.Type == tokenizer.LeftParenToken {
			p.consume(1)
			prefix := remaining + next.Value
			if len(p.buf) > 0 && hasStructuralWhitespace(p.buf[0]) {
				tok := p.buf[0]
				p.consume(1)
				p.lineStart = false
				events := []Event{{Type: BulletItemEvent, Value: prefix + " "}}
				if tok.Type == tokenizer.TabToken {
					p.contentIndent = tabRemainingEquiv(1)
				} else if tok.Type == tokenizer.TextToken {
					trimmed := strings.TrimPrefix(tok.Value, " ")
					if trimmed != "" {
						events = append(events, Event{Type: TextEvent, Value: trimmed})
					}
				}
				return events
			}
			p.lineStart = false
			return []Event{{Type: BulletItemEvent, Value: prefix + " "}}
		}
		if next.Type == tokenizer.TextToken && len(next.Value) > 0 && next.Value[0] == '.' {
			dot := next.Value
			if len(dot) > 1 && dot[1] == ' ' {
				p.consume(1)
				prefix := remaining + dot
				p.lineStart = false
				events := []Event{{Type: BulletItemEvent, Value: prefix}}
				rest := dot[len(prefix)-len(remaining):]
				if rest != "" {
					events = append(events, Event{Type: TextEvent, Value: rest})
				}
				return events
			}
			if len(dot) == 1 && len(p.buf) > 1 && hasStructuralWhitespace(p.buf[1]) {
				p.consume(1)
				tok := p.buf[0]
				p.consume(1)
				p.lineStart = false
				events := []Event{{Type: BulletItemEvent, Value: remaining + ". "}}
				if tok.Type == tokenizer.TabToken {
					p.contentIndent = tabRemainingEquiv(1)
				} else if tok.Type == tokenizer.TextToken {
					trimmed := strings.TrimPrefix(tok.Value, " ")
					if trimmed != "" {
						events = append(events, Event{Type: TextEvent, Value: trimmed})
					}
				}
				return events
			}
		}
	}
	return nil
}

func (p *Parser) tryImage() ([]Event, bool) {
	if len(p.buf) < 2 {
		return nil, false
	}
	tok := p.buf[0]
	if tok.Type != tokenizer.TextToken {
		return nil, false
	}
	val := tok.Value
	if !strings.HasSuffix(val, "!") {
		return nil, false
	}
	if p.buf[1].Type != tokenizer.LeftBracketToken {
		return nil, false
	}
	if len(val) >= 2 {
		prev := val[len(val)-2]
		if prev == '<' || prev == '!' {
			return nil, false
		}
	}
	var events []Event
	if len(val) > 1 {
		events = append(events, Event{Type: TextEvent, Value: val[:len(val)-1]})
	}
	p.consume(1)
	p.consume(1)
	p.lineStart = false
	p.linkParser.startImageText()
	return events, true
}

var emailAutolinkRe = regexp.MustCompile(
	`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:` +
		`[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.` +
		`[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

func (p *Parser) tryAutolink() ([]Event, bool) {
	if len(p.buf) < 2 {
		return nil, false
	}
	tok := p.buf[0]
	if tok.Type != tokenizer.TextToken {
		return nil, false
	}
	val := tok.Value
	ltIdx := strings.Index(val, "<")
	if ltIdx == -1 {
		return nil, false
	}
	gtIdx := -1
	for i := 1; i < len(p.buf); i++ {
		bt := p.buf[i]
		if bt.Type == tokenizer.NewlineToken {
			return nil, false
		}
		if bt.Type == tokenizer.GreaterToken {
			gtIdx = i
			break
		}
	}
	if gtIdx == -1 {
		return nil, false
	}

	var content strings.Builder
	content.WriteString(val[ltIdx+1:])
	for i := 1; i < gtIdx; i++ {
		content.WriteString(p.buf[i].Value)
	}
	contentStr := content.String()

	var events []Event
	if ltIdx > 0 {
		events = append(events, Event{Type: TextEvent, Value: val[:ltIdx]})
	}

	if isValidAutolinkURI(contentStr) {
		p.consume(1)
		for i := 1; i < gtIdx; i++ {
			p.consume(1)
		}
		p.consume(1)
		p.lineStart = false
		events = append(events, Event{Type: AutolinkURLEvent, Value: contentStr, URL: contentStr})
		if len(contentStr) > 0 {
			p.prevChar = contentStr[len(contentStr)-1]
		}
		return events, true
	}

	if emailAutolinkRe.MatchString(contentStr) {
		p.consume(1)
		for i := 1; i < gtIdx; i++ {
			p.consume(1)
		}
		p.consume(1)
		p.lineStart = false
		url := "mailto:" + contentStr
		events = append(events, Event{Type: AutolinkEmailEvent, Value: contentStr, URL: url})
		if len(contentStr) > 0 {
			p.prevChar = contentStr[len(contentStr)-1]
		}
		return events, true
	}

	return nil, false
}

func isValidAutolinkURI(s string) bool {
	if s == "" {
		return false
	}
	colon := strings.Index(s, ":")
	if colon < 2 || colon > 32 {
		return false
	}
	scheme := s[:colon]
	if !isASCIILetter(scheme[0]) {
		return false
	}
	for i := 1; i < len(scheme); i++ {
		b := scheme[i]
		if !isASCIILetter(b) && !isDigit(b) && b != '+' && b != '.' && b != '-' {
			return false
		}
	}
	for _, c := range s[colon+1:] {
		if c <= 0x1F || c == 0x7F || c == ' ' || c == '<' || c == '>' {
			return false
		}
	}
	return true
}

func (p *Parser) emitTextOrSpecial() []Event {
	var events []Event
	for len(p.buf) > 0 {
		tok := p.buf[0]
		switch tok.Type {
		case tokenizer.TextToken:
			p.consume(1)
			p.lineStart = false
			events = append(events, Event{Type: TextEvent, Value: tok.Value})
			if len(tok.Value) > 0 {
				p.prevChar = tok.Value[len(tok.Value)-1]
			}
		case tokenizer.NewlineToken:
			p.consume(1)
			p.lineStart = true
			events = append(events, Event{Type: NewlineEvent})
			p.prevChar = '\n'
		default:
			p.consume(1)
			p.lineStart = false
			events = append(events, Event{Type: TextEvent, Value: tok.Value})
			if len(tok.Value) > 0 {
				p.prevChar = tok.Value[len(tok.Value)-1]
			}
		}
		if p.state != NormalState || p.bufferHasPattern() {
			break
		}
	}
	return events
}
