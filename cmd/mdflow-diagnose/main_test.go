//go:build diagnose

package main

import (
	"testing"

	"github.com/cjccjj/mdflow/internal/commonmark"
	"github.com/cjccjj/mdflow/pkg/markdown/parser"
)

func TestSignatureNormalizesOperationFamilies(t *testing.T) {
	trace := []parser.TraceTransition{{
		Branch:   "state.link_url",
		PreState: "link_url",
		Outcome:  "progress",
	}}

	tests := []struct {
		name     string
		expected *commonmark.Operation
		actual   *commonmark.Operation
		want     string
	}{
		{
			name:     "emphasis markers",
			expected: &commonmark.Operation{Kind: "em_start"},
			actual:   &commonmark.Operation{Kind: "strong_end"},
			want:     "state.link_url | link_url | emphasis_marker | emphasis_marker | progress",
		},
		{
			name:     "resource links",
			expected: &commonmark.Operation{Kind: "link", Value: "label", URL: "/destination"},
			actual:   &commonmark.Operation{Kind: "image", Value: "alt", URL: "/image"},
			want:     "state.link_url | link_url | resource_link | resource_link | progress",
		},
		{
			name:     "code block operations",
			expected: &commonmark.Operation{Kind: "code_lang", Value: "go"},
			actual:   &commonmark.Operation{Kind: "code_block_end"},
			want:     "state.link_url | link_url | code_block | code_block | progress",
		},
		{
			name:     "ordinary kinds retain only their kind",
			expected: &commonmark.Operation{Kind: "text", Value: "foo"},
			actual:   &commonmark.Operation{Kind: "heading_start", Level: 2},
			want:     "state.link_url | link_url | text | heading_start | progress",
		},
		{
			name: "missing operations remain end marker",
			want: "state.link_url | link_url | <end> | <end> | progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := &commonmark.Difference{Expected: tt.expected, Actual: tt.actual}
			if got := signature(trace, 0, diff); got != tt.want {
				t.Fatalf("signature: got %q, want %q", got, tt.want)
			}
		})
	}
}
