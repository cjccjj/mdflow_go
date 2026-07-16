package parser

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/cjccjj/mdflow/internal/commonmark"
	"github.com/cjccjj/mdflow/pkg/markdown/tokenizer"
)

// These are protected cases promoted from the diagnostic inventory. They are
// intentionally separate from the full smoke suite: a future parser change
// must keep these measured compatibility improvements event-equivalent.
func TestProtectedCommonMarkDelimiterRunCases(t *testing.T) {
	assertProtectedSpecCases(t, []int{391, 427, 467, 468, 470})
}

func TestProtectedCommonMarkEmptyListItemCases(t *testing.T) {
	assertProtectedSpecCases(t, []int{282, 283})
}

func assertProtectedSpecCases(t *testing.T, numbers []int) {
	t.Helper()
	examples, err := commonmark.Load("../../../dev_docs/commonMark_spec.txt")
	if err != nil {
		t.Fatalf("load CommonMark fixtures: %v", err)
	}
	byNumber := make(map[int]commonmark.Example, len(examples))
	for _, example := range examples {
		byNumber[example.Number] = example
	}

	for _, number := range numbers {
		example, ok := byNumber[number]
		if !ok {
			t.Fatalf("fixture %d is missing", number)
		}
		expected := commonmark.ExpectedProjection(example.Markdown, example.HTML)
		if !expected.Comparable {
			t.Fatalf("fixture %d unexpectedly non-comparable: %s", number, expected.Reason)
		}

		for _, split := range []struct {
			name      string
			positions []int
		}{
			{name: "one-shot"},
			{name: "line", positions: commonmark.LineBoundarySplits(example.Markdown)},
			{name: "arbitrary-rune", positions: commonmark.RuneBoundarySplits(example.Markdown)},
		} {
			t.Run(fmt.Sprintf("ex%d/%s", number, split.name), func(t *testing.T) {
				actual := commonmark.ActualProjection(parseWithPositions(example.Markdown, split.positions))
				if !actual.Comparable {
					t.Fatalf("actual projection is non-comparable: %s", actual.Reason)
				}
				if diff := commonmark.FirstDifference(expected.Operations, actual.Operations); diff != nil {
					t.Fatalf("semantic mismatch: %s\nexpected: %#v\nactual: %#v", protectedDifference(diff), expected.Operations, actual.Operations)
				}
			})
		}
	}
}

func TestProtectedDelimiterRunVariants(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "minimized double strong",
			input: "____x____",
			want:  []string{"BoldStart", "BoldStart", "Text", "BoldEnd", "BoldEnd"},
		},
		{
			name:  "triple strong at line start",
			input: "******x******",
			want:  []string{"BoldStart", "BoldStart", "BoldStart", "Text", "BoldEnd", "BoldEnd", "BoldEnd"},
		},
		{
			name:  "nested strong",
			input: "__outer __inner__ outer__",
			want:  []string{"BoldStart", "Text", "BoldStart", "Text", "BoldEnd", "Text", "BoldEnd"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventTypes(parseWithPositions(tt.input, nil)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("event types: got %v, want %v", got, tt.want)
			}
		})
	}

	for _, input := range []string{"** x **", `\*`} {
		t.Run("delimiter whitespace or escape remains literal/unstyled "+input, func(t *testing.T) {
			for _, typ := range eventTypes(parseWithPositions(input, commonmark.RuneBoundarySplits(input))) {
				if typ == "BoldStart" || typ == "ItalicStart" {
					t.Fatalf("unexpected emphasis event %s for %q", typ, input)
				}
			}
		})
	}
}

func TestProtectedEmptyListItemVariants(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		want   []string
	}{
		{"dash minimized reproducer", []string{"-", "\n"}, []string{"BulletItem", "Newline"}},
		{"star empty item", []string{"*", "\n"}, []string{"BulletItem", "Newline"}},
		{"marker whitespace", []string{"- ", "\n"}, []string{"BulletItem", "Newline"}},
		{"escaped marker remains literal", []string{`\-`, "\n"}, []string{"Text", "Newline"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			var events []Event
			for _, chunk := range tt.chunks {
				events = append(events, p.Parse(tokenizer.Tokenize([]byte(chunk)))...)
			}
			events = append(events, p.Flush()...)
			events = append(events, p.CloseStates()...)
			if got := eventTypes(events); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("event types: got %v, want %v", got, tt.want)
			}
		})
	}
}

func parseWithPositions(input string, positions []int) []Event {
	p := New()
	var events []Event
	start := 0
	for _, position := range positions {
		if position <= start || position >= len(input) {
			continue
		}
		events = append(events, p.Parse(tokenizer.Tokenize([]byte(input[start:position])))...)
		start = position
	}
	if start < len(input) {
		events = append(events, p.Parse(tokenizer.Tokenize([]byte(input[start:])))...)
	}
	events = append(events, p.Flush()...)
	events = append(events, p.CloseStates()...)
	return events
}

func protectedDifference(diff *commonmark.Difference) string {
	expected := "<end>"
	actual := "<end>"
	if diff.Expected != nil {
		expected = diff.Expected.String()
	}
	if diff.Actual != nil {
		actual = diff.Actual.String()
	}
	return fmt.Sprintf("op[%d]: expected %s; actual %s", diff.Index, expected, actual)
}
