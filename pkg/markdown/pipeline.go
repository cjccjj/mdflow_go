package markdown

import (
	"bytes"
	"io"

	"github.com/cjccjj/mdflow/pkg/markdown/event"
	"github.com/cjccjj/mdflow/pkg/markdown/parser"
	"github.com/cjccjj/mdflow/pkg/markdown/render"
	"github.com/cjccjj/mdflow/pkg/markdown/tokenizer"
)

type Pipeline struct {
	parser  *parser.Parser
	writer  *render.Writer
	chunker streamChunker
}

func NewPipeline(w io.Writer, theme render.Theme) *Pipeline {
	aw := render.NewAnsiWriter(w)
	wr := render.NewWriter(aw, theme)
	wr.SetInlineRenderer(func(text string) string { return renderInlineWithParser(text, theme) })
	wr.SetLive(render.IsTerminal(w))
	wr.SetTermWidth(render.TerminalWidth(w))
	return &Pipeline{
		parser: parser.New(),
		writer: wr,
	}
}

func (p *Pipeline) Write(data []byte) (int, error) {
	n := len(data)
	return n, p.chunker.Write(data, p.parseChunk)
}

func (p *Pipeline) parseChunk(chunk []byte) error {
	return p.emit(p.parser.Parse(tokenizer.Tokenize(chunk)))
}

func (p *Pipeline) emit(events []event.Event) error {
	for _, e := range events {
		if err := p.writer.Handle(e); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pipeline) Flush() error {
	return p.emit(p.parser.Flush())
}

func (p *Pipeline) Reset() {
	p.writer.ResetStyles()
	p.parser.Reset()
	p.chunker.Reset()
}

// renderInlineWithParser is the pipeline-owned parser service used by tables.
func renderInlineWithParser(text string, theme render.Theme) string {
	var buf bytes.Buffer
	w := render.NewWriter(render.NewAnsiWriter(&buf), theme)
	p := parser.New()
	events := p.Parse(tokenizer.Tokenize([]byte(text)))
	events = append(events, p.CloseStates()...)
	for _, e := range events {
		if err := w.Handle(e); err != nil {
			return buf.String()
		}
	}
	return buf.String()
}

func (p *Pipeline) Close() error {
	if err := p.chunker.Drain(p.parseChunk); err != nil {
		return err
	}

	if err := p.Flush(); err != nil {
		return err
	}
	if err := p.emit(p.parser.CloseStates()); err != nil {
		return err
	}
	p.Reset()
	return nil
}
