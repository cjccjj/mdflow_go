package parser

import (
	"testing"

	"github.com/cjccjj/mdflow/pkg/markdown/tokenizer"
)

func autolinkEventTypes(input string) []string {
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

func autolinkEventValues(input string) []Event {
	p := New()
	tokens := tokenizer.Tokenize([]byte(input))
	p.eof = true
	events := p.Parse(tokens)
	events = append(events, p.CloseStates()...)
	return events
}

func TestAutolink_BasicURI(t *testing.T) {
	types := autolinkEventTypes("<http://foo.bar>")
	if len(types) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(types), types)
	}
	if types[0] != "AutolinkURL" {
		t.Errorf("expected AutolinkURL, got %s", types[0])
	}
}

func TestAutolink_URIValueAndURL(t *testing.T) {
	events := autolinkEventValues("<http://foo.bar>")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[0].Type.String())
	}
	if events[0].Value != "http://foo.bar" {
		t.Errorf("expected Value='http://foo.bar', got '%s'", events[0].Value)
	}
	if events[0].URL != "http://foo.bar" {
		t.Errorf("expected URL='http://foo.bar', got '%s'", events[0].URL)
	}
}

func TestAutolink_HTTPS(t *testing.T) {
	events := autolinkEventValues("<https://example.com/path?q=1>")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[0].Type.String())
	}
	if events[0].Value != "https://example.com/path?q=1" {
		t.Errorf("unexpected value: %s", events[0].Value)
	}
}

func TestAutolink_CustomScheme(t *testing.T) {
	events := autolinkEventValues("<irc://foo.bar:2233/baz>")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[0].Type.String())
	}
}

func TestAutolink_UppercaseScheme(t *testing.T) {
	events := autolinkEventValues("<HTTPS://foo>")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[0].Type.String())
	}
}

func TestAutolink_SchemeWithPlus(t *testing.T) {
	events := autolinkEventValues("<a+b+c:d>")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[0].Type.String())
	}
}

func TestAutolink_LocalhostURI(t *testing.T) {
	events := autolinkEventValues("<localhost:5001/foo>")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[0].Type.String())
	}
}

func TestAutolink_BasicEmail(t *testing.T) {
	events := autolinkEventValues("<foo@bar.example.com>")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != AutolinkEmailEvent {
		t.Errorf("expected AutolinkEmailEvent, got %s", events[0].Type.String())
	}
	if events[0].Value != "foo@bar.example.com" {
		t.Errorf("expected Value='foo@bar.example.com', got '%s'", events[0].Value)
	}
	if events[0].URL != "mailto:foo@bar.example.com" {
		t.Errorf("expected URL='mailto:foo@bar.example.com', got '%s'", events[0].URL)
	}
}

func TestAutolink_EmailWithSpecialChars(t *testing.T) {
	events := autolinkEventValues("<foo+special@Bar.baz-bar0.com>")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != AutolinkEmailEvent {
		t.Errorf("expected AutolinkEmailEvent, got %s", events[0].Type.String())
	}
}

func TestAutolink_TextBeforeAutolink(t *testing.T) {
	events := autolinkEventValues("before<http://foo>")
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(events), events)
	}
	if events[0].Type != TextEvent || events[0].Value != "before" {
		t.Errorf("expected TextEvent 'before', got %s '%s'", events[0].Type.String(), events[0].Value)
	}
	if events[1].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[1].Type.String())
	}
}

func TestAutolink_CrossTokenContent(t *testing.T) {
	// <http://foo&bar> — & is AmpersandToken, splits content across tokens
	events := autolinkEventValues("<http://foo&bar>")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(events), events)
	}
	if events[0].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[0].Type.String())
	}
	if events[0].Value != "http://foo&bar" {
		t.Errorf("expected 'http://foo&bar', got '%s'", events[0].Value)
	}
}

func TestAutolink_NotAutolink_Empty(t *testing.T) {
	events := autolinkEventValues("<>")
	for _, e := range events {
		if e.Type == AutolinkURLEvent || e.Type == AutolinkEmailEvent {
			t.Errorf("empty <> should not be autolink, got %s", e.Type.String())
		}
	}
}

func TestAutolink_NotAutolink_SpaceInContent(t *testing.T) {
	events := autolinkEventValues("< https://foo >")
	for _, e := range events {
		if e.Type == AutolinkURLEvent {
			t.Errorf("space in autolink content should not be valid URI, got %s", e.Type.String())
		}
	}
}

func TestAutolink_NotAutolink_ShortScheme(t *testing.T) {
	// <m:abc> — scheme "m" is 1 char, must be >=2
	events := autolinkEventValues("<m:abc>")
	for _, e := range events {
		if e.Type == AutolinkURLEvent {
			t.Errorf("1-char scheme should not be valid URI, got %s", e.Type.String())
		}
	}
}

func TestAutolink_NotAutolink_NoColon(t *testing.T) {
	// <foo.bar.baz> — no colon, not a URI. Also not an email (no @).
	events := autolinkEventValues("<foo.bar.baz>")
	for _, e := range events {
		if e.Type == AutolinkURLEvent || e.Type == AutolinkEmailEvent {
			t.Errorf("no-scheme content should not be autolink, got %s", e.Type.String())
		}
	}
}

func TestAutolink_NotAutolink_BareURIWithoutBrackets(t *testing.T) {
	// https://example.com — bare URL without <> is not an autolink
	events := autolinkEventValues("https://example.com")
	for _, e := range events {
		if e.Type == AutolinkURLEvent {
			t.Errorf("bare URL without <> should not be autolink, got %s", e.Type.String())
		}
	}
}

func TestAutolink_NotAutolink_BareEmailWithoutBrackets(t *testing.T) {
	events := autolinkEventValues("foo@bar.com")
	for _, e := range events {
		if e.Type == AutolinkEmailEvent {
			t.Errorf("bare email without <> should not be autolink, got %s", e.Type.String())
		}
	}
}

func TestAutolink_NotAutolink_BackslashEscaped(t *testing.T) {
	// \<http://foo> — backslash escapes the <
	events := autolinkEventValues(`\<http://foo>`)
	for _, e := range events {
		if e.Type == AutolinkURLEvent {
			t.Errorf("backslash-escaped < should not trigger autolink, got %s", e.Type.String())
		}
	}
}

func TestAutolink_NotAutolink_NewlineBeforeClose(t *testing.T) {
	p := New()
	p.eof = true
	tokens := tokenizer.Tokenize([]byte("<http://foo\n"))
	events := p.Parse(tokens)
	events = append(events, p.CloseStates()...)
	for _, e := range events {
		if e.Type == AutolinkURLEvent || e.Type == AutolinkEmailEvent {
			t.Errorf("newline before > should not produce autolink, got %s", e.Type.String())
		}
	}
}

func TestAutolink_HTMLBlockNotAutolink(t *testing.T) {
	// <![CDATA[hello]]> should be HTML block, not autolink
	events := autolinkEventValues("<![CDATA[hello]]>")
	for _, e := range events {
		if e.Type == AutolinkURLEvent || e.Type == AutolinkEmailEvent {
			t.Errorf("HTML CDATA should not be autolink, got %s", e.Type.String())
		}
	}
}

func TestAutolink_StreamingAcrossChunks(t *testing.T) {
	p := New()

	// Chunk 1: <http://foo without > — no autolink yet, emits as text
	events := p.Parse(tokenizer.Tokenize([]byte("<http://foo")))
	if len(events) != 1 {
		t.Fatalf("chunk 1: expected 1 text event, got %d: %v", len(events), events)
	}
	if events[0].Type != TextEvent || events[0].Value != "<http://foo" {
		t.Errorf("chunk 1: expected text '<http://foo', got %v", events[0])
	}

	// Chunk 2: closing > arrives — treated as text (autolink needs < and > together)
	p.eof = true
	events = p.Parse(tokenizer.Tokenize([]byte(">")))
	events = append(events, p.CloseStates()...)
	if len(events) != 1 {
		t.Fatalf("chunk 2: expected 1 text event, got %d: %v", len(events), events)
	}
	if events[0].Type != TextEvent || events[0].Value != ">" {
		t.Errorf("chunk 2: expected text '>', got %v", events[0])
	}
}

func TestAutolink_CompleteInOneChunk(t *testing.T) {
	// Full <http://foo> in one Parse call works
	p := New()
	p.eof = true
	events := p.Parse(tokenizer.Tokenize([]byte("<http://foo>")))
	events = append(events, p.CloseStates()...)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(events), events)
	}
	if events[0].Type != AutolinkURLEvent {
		t.Errorf("expected AutolinkURLEvent, got %s", events[0].Type.String())
	}
}

func TestAutolink_StreamingEmail(t *testing.T) {
	p := New()

	events := p.Parse(tokenizer.Tokenize([]byte("<foo@")))
	if len(events) != 1 {
		t.Fatalf("chunk 1: expected 1 text event, got %d: %v", len(events), events)
	}

	p.eof = true
	events = p.Parse(tokenizer.Tokenize([]byte("bar.com>")))
	events = append(events, p.CloseStates()...)
	if len(events) != 2 {
		t.Fatalf("chunk 2: expected 2 text events, got %d: %v", len(events), events)
	}
	for _, e := range events {
		if e.Type == AutolinkEmailEvent {
			t.Errorf("cross-chunk should not produce autolink, got %s", e.Type.String())
		}
	}
}

func TestAutolink_NotLineStartAfterAutolink(t *testing.T) {
	// After an autolink, the parser should not be in line-start mode.
	// <http://foo># heading should render as autolink + text, not H1 heading.
	events := autolinkEventValues("<http://foo># heading")
	foundHeader := false
	for _, e := range events {
		if e.Type == HeaderStartEvent {
			foundHeader = true
		}
	}
	if foundHeader {
		t.Error("# heading after autolink should not be parsed as heading")
	}
}
