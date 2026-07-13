package render

import (
	"bytes"
	"testing"

	"github.com/cjccjj/mdflow/pkg/markdown/event"
)

func TestWriterUsesInjectedInlineRendererForTables(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(NewAnsiWriter(&buf), DefaultTheme)
	calls := 0
	w.SetInlineRenderer(func(text string) string {
		calls++
		return "rendered-" + text
	})
	if err := w.Handle(event.Event{Type: event.TableStartEvent, Cells: []string{"head"}, Widths: []int{3}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Handle(event.Event{Type: event.TableRowEvent, Cells: []string{"cell"}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Handle(event.Event{Type: event.TableEndEvent}); err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("injected inline renderer was not used")
	}
}
