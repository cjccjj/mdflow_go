package parser

import (
	"reflect"
	"testing"
)

// TestCoreEventContract is mdflow-owned, exact-event coverage for capabilities
// documented as supported. CommonMark's HTML examples remain a diagnostic
// oracle; this table is the mandatory streaming parser contract.
func TestCoreEventContract(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		want   []Event
	}{
		{
			name:   "atx heading",
			chunks: []string{"# title\n"},
			want:   []Event{{Type: HeaderStartEvent, Level: 1}, {Type: TextEvent, Value: "title"}, {Type: HeaderEndEvent}, {Type: NewlineEvent}},
		},
		{
			name:   "setext heading",
			chunks: []string{"title\n", "---\n"},
			want:   []Event{{Type: HeaderStartEvent, Level: 2}, {Type: TextEvent, Value: "title"}, {Type: HeaderEndEvent}, {Type: NewlineEvent}},
		},
		{
			name:   "emphasis and strikethrough",
			chunks: []string{"**bold** *italic* ~~strike~~"},
			want: []Event{
				{Type: BoldStartEvent}, {Type: TextEvent, Value: "bold"}, {Type: BoldEndEvent}, {Type: TextEvent, Value: " "},
				{Type: ItalicStartEvent}, {Type: TextEvent, Value: "italic"}, {Type: ItalicEndEvent}, {Type: TextEvent, Value: " "},
				{Type: StrikethroughStartEvent}, {Type: TextEvent, Value: "strike"}, {Type: StrikethroughEndEvent},
			},
		},
		{
			name:   "inline code",
			chunks: []string{"use `code`"},
			want:   []Event{{Type: TextEvent, Value: "use "}, {Type: InlineCodeStartEvent}, {Type: TextEvent, Value: "code"}, {Type: InlineCodeEndEvent}},
		},
		{
			name:   "fenced code with language",
			chunks: []string{"```go\ncode\n```\n"},
			want: []Event{
				{Type: CodeBlockStartEvent}, {Type: CodeBlockLangEvent, Value: "go"}, {Type: NewlineEvent}, {Type: TextEvent, Value: "code"}, {Type: NewlineEvent}, {Type: CodeBlockEndEvent}, {Type: NewlineEvent},
			},
		},
		{
			name:   "indented code eof closes",
			chunks: []string{"    code\n"},
			want:   []Event{{Type: CodeBlockStartEvent}, {Type: TextEvent, Value: "code"}, {Type: NewlineEvent}, {Type: CodeBlockEndEvent}},
		},
		{
			name:   "flat and empty list items",
			chunks: []string{"- item\n-\n1. first\n"},
			want: []Event{
				{Type: BulletItemEvent}, {Type: TextEvent, Value: "item"}, {Type: NewlineEvent},
				{Type: BulletItemEvent}, {Type: NewlineEvent},
				{Type: BulletItemEvent, Value: "1. "}, {Type: TextEvent, Value: "first"}, {Type: NewlineEvent},
			},
		},
		{
			name:   "thematic break",
			chunks: []string{"---\n"},
			want:   []Event{{Type: HorizontalRuleEvent}, {Type: NewlineEvent}},
		},
		{
			name:   "blockquote eof boundary",
			chunks: []string{"> quoted\n"},
			want:   []Event{{Type: BlockquoteStartEvent}, {Type: TextEvent, Value: "quoted"}, {Type: BlockquoteEndEvent}},
		},
		{
			name:   "gfm table",
			chunks: []string{"| A | B |\n", "| --- | :---: |\n", "| 1 | 2 |\n"},
			want: []Event{
				{Type: TableStartEvent, Cells: []string{"A", "B"}, Widths: []int{3, 5}, Aligns: []int{0, 1}},
				{Type: TableRowEvent, Cells: []string{"1", "2"}}, {Type: TableEndEvent},
			},
		},
		{
			name:   "inline link and image",
			chunks: []string{"[label](/url \"title\")", "![alt](/image)"},
			want: []Event{
				{Type: LinkEvent, Value: "label", URL: "/url", Title: "title"}, {Type: ImageEvent, Value: "alt", URL: "/image"},
			},
		},
		{
			name:   "autolinks",
			chunks: []string{"<https://example.test> <me@example.test>"},
			want: []Event{
				{Type: AutolinkURLEvent, Value: "https://example.test", URL: "https://example.test"}, {Type: TextEvent, Value: " "},
				{Type: AutolinkEmailEvent, Value: "me@example.test", URL: "mailto:me@example.test"},
			},
		},
		{
			name:   "escape entity and inline html stripping",
			chunks: []string{"\\*&amp;<b>x</b>"},
			want:   []Event{{Type: TextEvent, Value: "*"}, {Type: TextEvent, Value: "&"}, {Type: TextEvent, Value: "x"}},
		},
		{
			name:   "literal fallback",
			chunks: []string{"-not a list\n[label]\n"},
			want: []Event{
				{Type: TextEvent, Value: "-"}, {Type: TextEvent, Value: "not a list"}, {Type: NewlineEvent},
				{Type: TextEvent, Value: "["}, {Type: TextEvent, Value: "label"}, {Type: TextEvent, Value: "]"}, {Type: NewlineEvent},
			},
		},
		{
			name:   "eof closes open emphasis and falls back literal code opener",
			chunks: []string{"**open `code"},
			want: []Event{
				{Type: BoldStartEvent}, {Type: TextEvent, Value: "open "}, {Type: TextEvent, Value: "`"}, {Type: TextEvent, Value: "code"}, {Type: BoldEndEvent},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCoreContract(tt.chunks)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("events:\n got %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

func parseCoreContract(chunks []string) []Event {
	p := New()
	var events []Event
	for _, chunk := range chunks {
		events = append(events, p.Parse(tokenize(chunk))...)
	}
	events = append(events, p.Flush()...)
	events = append(events, p.CloseStates()...)
	return events
}
