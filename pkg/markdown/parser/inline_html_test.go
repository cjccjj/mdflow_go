package parser

import (
	"strings"
	"testing"

	"github.com/cjccjj/mdflow/pkg/markdown/tokenizer"
)

func inlineHTMLEventTypes(input string) []string {
	p := New()
	tokens := tokenizer.Tokenize([]byte(input))
	p.eof = true
	events := p.Parse(tokens)
	events = append(events, p.CloseStates()...)
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.Type.String()
	}
	return types
}

func inlineHTMLEventValues(input string) []Event {
	p := New()
	tokens := tokenizer.Tokenize([]byte(input))
	p.eof = true
	events := p.Parse(tokens)
	events = append(events, p.CloseStates()...)
	return events
}

func inlineHTMLAllText(input string) string {
	events := inlineHTMLEventValues(input)
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString(e.Value)
	}
	return sb.String()
}

func TestInlineHTML_BasicOpenTag(t *testing.T) {
	text := inlineHTMLAllText("<b>text</b>")
	if text != "text" {
		t.Errorf("expected 'text', got %q", text)
	}
}

func TestInlineHTML_BasicOpenTagMixed(t *testing.T) {
	text := inlineHTMLAllText("hello<b>world</b>")
	if text != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", text)
	}
}

func TestInlineHTML_SelfClosingTag(t *testing.T) {
	text := inlineHTMLAllText("<br/>")
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_SelfClosingWithSpace(t *testing.T) {
	text := inlineHTMLAllText("<br />")
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_TagWithAttributes(t *testing.T) {
	text := inlineHTMLAllText(`<a href="http://x.com">link</a>`)
	if text != "link" {
		t.Errorf("expected 'link', got %q", text)
	}
}

func TestInlineHTML_TagWithGreaterInQuotedAttr(t *testing.T) {
	text := inlineHTMLAllText(`<img alt="a > b">`)
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_TagWithSingleAndDoubleQuotes(t *testing.T) {
	text := inlineHTMLAllText(`<img alt='a > b' title="c > d">`)
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_ClosingTag(t *testing.T) {
	text := inlineHTMLAllText("</div>")
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_ClosingTagWithSpace(t *testing.T) {
	text := inlineHTMLAllText("</div >")
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_Comment(t *testing.T) {
	text := inlineHTMLAllText("before<!-- comment -->after")
	if text != "beforeafter" {
		t.Errorf("expected 'beforeafter', got %q", text)
	}
}

func TestInlineHTML_CommentShort(t *testing.T) {
	text := inlineHTMLAllText("<!-->")
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_CommentDashes(t *testing.T) {
	text := inlineHTMLAllText("<!--->")
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_ProcessingInstruction(t *testing.T) {
	text := inlineHTMLAllText("<?php echo $a; ?>")
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_Declaration(t *testing.T) {
	text := inlineHTMLAllText("<!DOCTYPE html>")
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_CDATA(t *testing.T) {
	text := inlineHTMLAllText("<![CDATA[>&<]]>")
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}

func TestInlineHTML_NotATag_LessThanSpace(t *testing.T) {
	text := inlineHTMLAllText("< a>")
	if text != "< a>" {
		t.Errorf("expected '< a>', got %q", text)
	}
}

func TestInlineHTML_NotATag_DigitTagName(t *testing.T) {
	text := inlineHTMLAllText("<33>")
	if text != "<33>" {
		t.Errorf("expected '<33>', got %q", text)
	}
}

func TestInlineHTML_NotATag_UnderscoreTagName(t *testing.T) {
	// <__> is not an HTML tag (underscore is not a valid tag name start).
	// The < and > should be emitted as text; the __ may be parsed as emphasis.
	events := inlineHTMLEventValues("<__>")
	hasLT := false
	hasGT := false
	for _, e := range events {
		if e.Type == TextEvent && e.Value == "<" {
			hasLT = true
		}
		if e.Type == TextEvent && e.Value == ">" {
			hasGT = true
		}
	}
	if !hasLT || !hasGT {
		t.Errorf("expected '<' and '>' as text, got %v", eventTypes(events))
	}
}

func TestInlineHTML_NotATag_Comparison(t *testing.T) {
	text := inlineHTMLAllText("3 < 5 > 0")
	expected := "3 < 5 > 0"
	if text != expected {
		t.Errorf("expected %q, got %q", expected, text)
	}
}

func TestInlineHTML_NotATag_Empty(t *testing.T) {
	text := inlineHTMLAllText("<>")
	if text != "<>" {
		t.Errorf("expected '<>', got %q", text)
	}
}

func TestInlineHTML_NotATag_ClosingWithAttributes(t *testing.T) {
	text := inlineHTMLAllText(`</a href="foo">before`)
	if text != "before" {
		t.Errorf("expected 'before', got %q", text)
	}
}

func TestInlineHTML_MultipleConsecutive(t *testing.T) {
	text := inlineHTMLAllText("<b>bold</b> and <i>italic</i>")
	if text != "bold and italic" {
		t.Errorf("expected 'bold and italic', got %q", text)
	}
}

func TestInlineHTML_AutolinkNotIntercepted(t *testing.T) {
	p := New()
	p.eof = true
	events := p.Parse(tokenizer.Tokenize([]byte("<http://foo.bar>")))
	events = append(events, p.CloseStates()...)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(events), eventTypes(events))
	}
	if events[0].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[0].Type.String())
	}
}

func TestInlineHTML_StreamingAcrossChunks(t *testing.T) {
	p := New()

	events := p.Parse(tokenizer.Tokenize([]byte("<div")))
	if len(events) != 0 {
		t.Fatalf("chunk 1: expected 0 events (pause), got %d: %v", len(events), eventTypes(events))
	}

	p.eof = true
	events = p.Parse(tokenizer.Tokenize([]byte(" class=\"foo\">text")))
	events = append(events, p.CloseStates()...)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(events), eventTypes(events))
	}
	if events[0].Type != TextEvent || events[0].Value != "text" {
		t.Errorf("expected text 'text', got %v", events[0])
	}
}

func TestInlineHTML_StreamingSplitAtLessThan(t *testing.T) {
	p := New()

	// Chunk 1: "before<" — can't determine tag type from just "<", pauses silently.
	events := p.Parse(tokenizer.Tokenize([]byte("before<")))
	if len(events) != 0 {
		t.Fatalf("chunk 1: expected 0 events (pause), got %d: %v", len(events), eventTypes(events))
	}

	// Chunk 2: "b>after" — now we see "<b>" which is an HTML tag, stripped.
	p.eof = true
	events = p.Parse(tokenizer.Tokenize([]byte("b>after")))
	events = append(events, p.CloseStates()...)
	if len(events) != 2 {
		t.Fatalf("chunk 2: expected 2 events (before + after), got %d: %v", len(events), eventTypes(events))
	}
	if events[0].Type != TextEvent || events[0].Value != "before" {
		t.Errorf("event 0: expected 'before', got %q", events[0].Value)
	}
	if events[1].Type != TextEvent || events[1].Value != "after" {
		t.Errorf("event 1: expected 'after', got %q", events[1].Value)
	}
}

func TestInlineHTML_TextWithMultipleTags(t *testing.T) {
	text := inlineHTMLAllText("a<b>c</b>d<i>e</i>f")
	if text != "acdef" {
		t.Errorf("expected 'acdef', got %q", text)
	}
}

func TestInlineHTML_CommentWithDashTokens(t *testing.T) {
	// <!-- -- is split by tokenizer into DashTokens, test that comment end is found
	text := inlineHTMLAllText("hello<!-- -- -->world")
	if text != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", text)
	}
}

func TestInlineHTML_CDATAWithBracketTokens(t *testing.T) {
	// ]]> — brackets are RightBracketToken and > is GreaterToken
	text := inlineHTMLAllText("<![CDATA[data]]>text")
	if text != "text" {
		t.Errorf("expected 'text', got %q", text)
	}
}

func TestInlineHTML_EOFUnclosedTag(t *testing.T) {
	// When EOF is set and a tag is incomplete, it should fall through as text
	events := inlineHTMLEventValues("<div")
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString(e.Value)
	}
	if !strings.Contains(sb.String(), "<div") {
		t.Errorf("expected '<div' to be emitted on EOF, got %q", sb.String())
	}
}

func TestInlineHTML_EOFUnclosedComment(t *testing.T) {
	events := inlineHTMLEventValues("<!--")
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString(e.Value)
	}
	if !strings.Contains(sb.String(), "<!--") {
		t.Errorf("expected '<!--' to be emitted on EOF, got %q", sb.String())
	}
}
