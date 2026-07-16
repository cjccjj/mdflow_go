package commonmark

import (
	"reflect"
	"testing"

	"github.com/cjccjj/mdflow/pkg/markdown/event"
)

func TestLoadReadsSourceSpec(t *testing.T) {
	examples, err := Load("../../dev_docs/commonMark_spec.txt")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(examples) < 650 {
		t.Fatalf("expected the full CommonMark fixture set, got %d examples", len(examples))
	}
	if examples[0].Number != 1 || examples[0].Markdown == "" || examples[0].HTML == "" {
		t.Fatalf("first fixture was not populated: %#v", examples[0])
	}
}

func TestExpectedProjectionSubset(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		html     string
		want     []Operation
	}{
		{
			name:     "heading with emphasis",
			markdown: "# *title*\n",
			html:     "<h1><em>title</em></h1>\n",
			want: []Operation{
				{Kind: "heading_start", Level: 1}, {Kind: "em_start"}, {Kind: "text", Value: "title"}, {Kind: "em_end"}, {Kind: "heading_end"}, {Kind: "newline"},
			},
		},
		{
			name:     "fenced code ignores serialization newline",
			markdown: "```go\ncode\n```\n",
			html:     "<pre><code class=\"language-go\">code\n</code></pre>\n",
			want: []Operation{
				{Kind: "code_block_start"}, {Kind: "code_lang", Value: "go"}, {Kind: "text", Value: "code"}, {Kind: "newline"}, {Kind: "code_block_end"},
			},
		},
		{
			name:     "link and image",
			markdown: "[link](/url) ![alt](/image)\n",
			html:     "<p><a href=\"/url\">link</a> <img src=\"/image\" alt=\"alt\" /></p>\n",
			want: []Operation{
				{Kind: "link", Value: "link", URL: "/url"}, {Kind: "text", Value: " "}, {Kind: "image", Value: "alt", URL: "/image"}, {Kind: "newline"},
			},
		},
		{
			name:     "blockquote boundary normalizes terminal order",
			markdown: "> foo\n---\n",
			html:     "<blockquote>\n<p>foo</p>\n</blockquote>\n<hr />\n",
			want: []Operation{
				{Kind: "blockquote_start"}, {Kind: "text", Value: "foo"}, {Kind: "newline"}, {Kind: "blockquote_end"}, {Kind: "thematic_break"}, {Kind: "newline"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpectedProjection(tt.markdown, tt.html)
			if !got.Comparable {
				t.Fatalf("expected comparable projection, got %q", got.Reason)
			}
			if !reflect.DeepEqual(got.Operations, tt.want) {
				t.Fatalf("operations:\n got %#v\nwant %#v", got.Operations, tt.want)
			}
		})
	}
}

func TestProjectionMarksUnsupportedInputs(t *testing.T) {
	for _, tt := range []struct {
		name     string
		markdown string
		html     string
	}{
		{"raw HTML", "<div>text</div>\n", "<div>text</div>\n"},
		{"nested list", "- one\n  - two\n", "<ul>\n<li>one\n<ul>\n<li>two</li>\n</ul>\n</li>\n</ul>\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpectedProjection(tt.markdown, tt.html)
			if got.Comparable || got.Reason == "" {
				t.Fatalf("expected non-comparable projection, got %#v", got)
			}
		})
	}
}

func TestActualProjectionCanonicalizesBlockquoteOrder(t *testing.T) {
	got := ActualProjection([]event.Event{
		{Type: event.BlockquoteStartEvent},
		{Type: event.TextEvent, Value: "foo"},
		{Type: event.BlockquoteEndEvent},
		{Type: event.NewlineEvent},
	})
	want := []Operation{
		{Kind: "blockquote_start"}, {Kind: "text", Value: "foo"}, {Kind: "newline"}, {Kind: "blockquote_end"},
	}
	if !got.Comparable || !reflect.DeepEqual(got.Operations, want) {
		t.Fatalf("projection:\n got %#v\nwant %#v", got, want)
	}
}
