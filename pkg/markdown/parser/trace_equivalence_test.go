//go:build diagnose

package parser_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/cjccjj/mdflow/pkg/markdown/parser"
	"github.com/cjccjj/mdflow/pkg/markdown/render"
	"github.com/cjccjj/mdflow/pkg/markdown/tokenizer"
)

// The enabled trace recorder is observational only. This verifies the parser
// event stream and the ANSI writer output against an otherwise identical
// parser with no recorder attached.
func TestTraceRecorderDoesNotChangeEventsOrRendering(t *testing.T) {
	for _, input := range []string{
		"# title\n**bold** and `code`\n",
		"____nested____\n",
		"-\n\n  paragraph\n",
		"[link](/url) <https://example.test>\n",
	} {
		t.Run(input, func(t *testing.T) {
			plain, _ := parseTraceComparable(input, false)
			traced, trace := parseTraceComparable(input, true)
			if len(trace.Transitions) == 0 {
				t.Fatal("expected trace transitions")
			}
			if !reflect.DeepEqual(plain, traced) {
				t.Fatalf("trace changed parser events:\nplain=%#v\ntraced=%#v", plain, traced)
			}
			if plainRendered, tracedRendered := renderTraceEvents(plain), renderTraceEvents(traced); plainRendered != tracedRendered {
				t.Fatalf("trace changed rendered output:\nplain=%q\ntraced=%q", plainRendered, tracedRendered)
			}
		})
	}
}

func parseTraceComparable(input string, enable bool) ([]parser.Event, *parser.Trace) {
	p := parser.New()
	var trace *parser.Trace
	if enable {
		trace = p.EnableTrace()
	}
	events := p.Parse(tokenizer.Tokenize([]byte(input)))
	events = append(events, p.Flush()...)
	events = append(events, p.CloseStates()...)
	return events, trace
}

func renderTraceEvents(events []parser.Event) string {
	var output bytes.Buffer
	w := render.NewWriter(render.NewAnsiWriter(&output), render.DefaultTheme)
	for _, event := range events {
		if err := w.Handle(event); err != nil {
			return err.Error()
		}
	}
	return output.String()
}
