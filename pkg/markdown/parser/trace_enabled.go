//go:build diagnose

package parser

import (
	"fmt"
	"strings"

	"github.com/cjccjj/mdflow/pkg/markdown/event"
)

// Trace is a debug-only record of parser dispatch transitions. Enable it by
// building with -tags diagnose; it is intentionally not a production library
// extension point.
type Trace struct {
	Transitions []TraceTransition `json:"transitions"`
}

// TraceTransition describes one existing process() dispatch iteration or one
// finalization operation. A transition may consume multiple buffered tokens.
type TraceTransition struct {
	Sequence         uint64        `json:"sequence"`
	PreState         string        `json:"pre_state"`
	PostState        string        `json:"post_state"`
	PreContext       TraceContext  `json:"pre_context"`
	PostContext      TraceContext  `json:"post_context"`
	PreSubstate      TraceSubstate `json:"pre_substate"`
	PostSubstate     TraceSubstate `json:"post_substate"`
	Branch           string        `json:"branch"`
	HeadTokenOrdinal uint64        `json:"head_token_ordinal"`
	HeadTokenType    string        `json:"head_token_type,omitempty"`
	HeadTokenPreview string        `json:"head_token_preview,omitempty"`
	BufferBefore     int           `json:"buffer_before"`
	BufferAfter      int           `json:"buffer_after"`
	BufferProgress   int           `json:"buffer_progress"`
	Events           []TraceEvent  `json:"events,omitempty"`
	Outcome          string        `json:"outcome"`
}

// TraceContext records parser state shared by block and inline branches.
type TraceContext struct {
	LineStart     bool   `json:"line_start"`
	PreviousChar  string `json:"previous_char,omitempty"`
	ContentIndent int    `json:"content_indent"`
	EOF           bool   `json:"eof"`
}

// TraceSubstate is intentionally concise: it makes buffered streaming
// decisions inspectable without exposing mutable parser internals.
type TraceSubstate struct {
	Link     string `json:"link,omitempty"`
	Emphasis string `json:"emphasis,omitempty"`
	Table    string `json:"table,omitempty"`
	Block    string `json:"block,omitempty"`
}

// TraceEvent is a stable JSON-friendly projection of an emitted event.
type TraceEvent struct {
	Type   string   `json:"type"`
	Value  string   `json:"value,omitempty"`
	Level  int      `json:"level,omitempty"`
	Cells  []string `json:"cells,omitempty"`
	Widths []int    `json:"widths,omitempty"`
	Aligns []int    `json:"aligns,omitempty"`
	URL    string   `json:"url,omitempty"`
	Title  string   `json:"title,omitempty"`
}

// EnableTrace activates collection for this parser instance and returns its
// debug-only trace. It must only be called from a diagnose build.
func (p *Parser) EnableTrace() *Trace {
	if p.trace == nil {
		p.trace = &traceRecorder{}
	}
	p.trace.trace = &Trace{}
	return p.trace.trace
}

type traceSnapshot struct {
	state       State
	context     TraceContext
	substate    TraceSubstate
	headOrdinal uint64
	headType    string
	headPreview string
	bufferLen   int
}

type traceRecorder struct {
	trace    *Trace
	sequence uint64
	pending  traceSnapshot
	branchID string
}

func (t *traceRecorder) begin(p *Parser) {
	if t == nil || t.trace == nil {
		return
	}
	t.pending = snapshotTrace(p)
	t.branchID = "dispatch.unknown"
}

func (t *traceRecorder) branch(branch string) {
	if t == nil || t.trace == nil {
		return
	}
	t.branchID = branch
}

func (t *traceRecorder) finish(p *Parser, events []Event, outcome string) {
	if t == nil || t.trace == nil {
		return
	}
	post := snapshotTrace(p)
	t.sequence++
	t.trace.Transitions = append(t.trace.Transitions, TraceTransition{
		Sequence:         t.sequence,
		PreState:         traceStateName(t.pending.state),
		PostState:        traceStateName(post.state),
		PreContext:       t.pending.context,
		PostContext:      post.context,
		PreSubstate:      t.pending.substate,
		PostSubstate:     post.substate,
		Branch:           t.branchID,
		HeadTokenOrdinal: t.pending.headOrdinal,
		HeadTokenType:    t.pending.headType,
		HeadTokenPreview: t.pending.headPreview,
		BufferBefore:     t.pending.bufferLen,
		BufferAfter:      post.bufferLen,
		BufferProgress:   t.pending.bufferLen - post.bufferLen,
		Events:           traceEvents(events),
		Outcome:          outcome,
	})
}

func (t *traceRecorder) finalize(p *Parser, mode finalizeMode, events []Event) {
	if t == nil || t.trace == nil {
		return
	}
	before := snapshotTrace(p)
	t.sequence++
	t.trace.Transitions = append(t.trace.Transitions, TraceTransition{
		Sequence:         t.sequence,
		PreState:         traceStateName(before.state),
		PostState:        traceStateName(p.state),
		PreContext:       before.context,
		PostContext:      snapshotTrace(p).context,
		PreSubstate:      before.substate,
		PostSubstate:     snapshotTrace(p).substate,
		Branch:           "finalize." + traceFinalizeMode(mode),
		HeadTokenOrdinal: before.headOrdinal,
		HeadTokenType:    before.headType,
		HeadTokenPreview: before.headPreview,
		BufferBefore:     before.bufferLen,
		BufferAfter:      p.bufferedLen(),
		BufferProgress:   0,
		Events:           traceEvents(events),
		Outcome:          "finalize",
	})
}

func snapshotTrace(p *Parser) traceSnapshot {
	s := traceSnapshot{
		state:       p.state,
		context:     TraceContext{LineStart: p.lineStart, ContentIndent: p.contentIndent, EOF: p.eof},
		substate:    traceSubstate(p),
		headOrdinal: p.headTokenOrdinal(),
		bufferLen:   p.bufferedLen(),
	}
	if p.prevChar != 0 {
		s.context.PreviousChar = preview(string(p.prevChar))
	}
	if len(p.buf) > 0 {
		s.headType = p.buf[0].Type.String()
		s.headPreview = preview(p.buf[0].Value)
	}
	return s
}

func traceSubstate(p *Parser) TraceSubstate {
	link := p.linkParser
	emph := p.emphasisParser
	table := p.tableParser
	block := p.blockParser
	return TraceSubstate{
		Link:     fmt.Sprintf("text=%d url=%d title=%d depth=%d parens=%d angle=%t done=%t image=%t", len(link.linkBuf), len(link.linkURLBuf), len(link.linkTitleBuf), link.linkDepth, link.urlParenDepth, link.urlAngleBracket, link.urlDone, link.isImage),
		Emphasis: fmt.Sprintf("depth=%d", emph.depth()),
		Table:    fmt.Sprintf("header=%d widths=%d aligns=%d", len(table.tableHeaderBuf), len(table.tableColWidths), len(table.tableColAligns)),
		Block:    fmt.Sprintf("header=%d fence=%s:%d first=%t indent=%d", block.headerLvl, block.fenceChar.String(), block.fenceLen, block.codeBlockFirst, block.codeBlockIndent),
	}
}

func traceEvents(events []Event) []TraceEvent {
	if len(events) == 0 {
		return nil
	}
	output := make([]TraceEvent, len(events))
	for i, e := range events {
		output[i] = TraceEvent{Type: event.Type(e.Type).String(), Value: e.Value, Level: e.Level, Cells: append([]string(nil), e.Cells...), Widths: append([]int(nil), e.Widths...), Aligns: append([]int(nil), e.Aligns...), URL: e.URL, Title: e.Title}
	}
	return output
}

func traceStateName(state State) string {
	switch state {
	case NormalState:
		return "normal"
	case HeaderState:
		return "header"
	case BoldState:
		return "bold_frame"
	case ItalicState:
		return "italic_frame"
	case StrikethroughState:
		return "strikethrough_frame"
	case InlineCodeState:
		return "inline_code"
	case CodeBlockState:
		return "code_block"
	case IndentedCodeBlockState:
		return "indented_code_block"
	case BlockquoteState:
		return "blockquote"
	case TablePendingState:
		return "table_pending"
	case TableBodyState:
		return "table_body"
	case SetextPendingState:
		return "setext_pending"
	case LinkTextState:
		return "link_text"
	case LinkURLState:
		return "link_url"
	case HTMLBlockState:
		return "html_block"
	case LinkRefDefState:
		return "link_ref_definition"
	default:
		return fmt.Sprintf("state_%d", state)
	}
}

func traceFinalizeMode(mode finalizeMode) string {
	switch mode {
	case finalizeClose:
		return "close"
	case finalizeFlush:
		return "flush"
	case finalizeSafe:
		return "recovery"
	default:
		return "unknown"
	}
}

func preview(value string) string {
	const limit = 48
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\t", "\\t")
	if len(value) > limit {
		return value[:limit] + "…"
	}
	return value
}
