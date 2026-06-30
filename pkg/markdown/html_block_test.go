package markdown

import (
	"strings"
	"testing"
)

func TestHTMLBlock_PreSingleLine(t *testing.T) {
	out := renderOutput("<pre>hello</pre>\n")
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello', got %q", out)
	}
	if strings.Contains(out, "<pre>") || strings.Contains(out, "</pre>") {
		t.Errorf("tags should be stripped: %q", out)
	}
}

func TestHTMLBlock_PreMultiLine(t *testing.T) {
	out := renderOutput("<pre>\nline 1\nline 2\n</pre>\n")
	if !strings.Contains(out, "line 1") {
		t.Errorf("expected 'line 1', got %q", out)
	}
	if !strings.Contains(out, "line 2") {
		t.Errorf("expected 'line 2', got %q", out)
	}
	if strings.Contains(out, "<pre>") || strings.Contains(out, "</pre>") {
		t.Errorf("tags should be stripped: %q", out)
	}
}

func TestHTMLBlock_Script(t *testing.T) {
	out := renderOutput("<script>\nalert('hi')\n</script>\n")
	if !strings.Contains(out, "alert('hi')") {
		t.Errorf("expected alert content, got %q", out)
	}
	if strings.Contains(out, "<script>") || strings.Contains(out, "</script>") {
		t.Errorf("tags should be stripped: %q", out)
	}
}

func TestHTMLBlock_Comment(t *testing.T) {
	out := renderOutput("<!-- this is a comment -->\n")
	if strings.Contains(out, "<!--") || strings.Contains(out, "-->") {
		t.Errorf("comment markup should be stripped: %q", out)
	}
	if strings.Contains(out, "this is a comment") {
		t.Errorf("comment text should not be visible: %q", out)
	}
}

func TestHTMLBlock_CommentBetweenParagraphs(t *testing.T) {
	out := renderOutput("before\n\n<!-- comment -->\n\nafter\n")
	if !strings.Contains(out, "before") {
		t.Errorf("expected 'before', got %q", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("expected 'after', got %q", out)
	}
	if strings.Contains(out, "<!--") {
		t.Errorf("comment tag should be stripped: %q", out)
	}
}

func TestHTMLBlock_ProcessingInstruction(t *testing.T) {
	out := renderOutput("<?xml version=\"1.0\"?>\n")
	if strings.Contains(out, "<?xml") {
		t.Errorf("PI should be stripped: %q", out)
	}
}

func TestHTMLBlock_Declaration(t *testing.T) {
	out := renderOutput("<!DOCTYPE html>\n")
	if strings.Contains(out, "<!DOCTYPE") {
		t.Errorf("declaration should be stripped: %q", out)
	}
}

func TestHTMLBlock_CDATA(t *testing.T) {
	out := renderOutput("<![CDATA[ raw data ]]>\n")
	if strings.Contains(out, "<![CDATA[") {
		t.Errorf("CDATA should be stripped: %q", out)
	}
}

func TestHTMLBlock_PreWithAttributes(t *testing.T) {
	out := renderOutput("<pre class=\"code\" id=\"x\">hello</pre>\n")
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello', got %q", out)
	}
	if strings.Contains(out, "<pre") || strings.Contains(out, "</pre>") {
		t.Errorf("tags should be stripped: %q", out)
	}
}

func TestHTMLBlock_Style(t *testing.T) {
	out := renderOutput("<style>\nbody { color: red; }\n</style>\n")
	if !strings.Contains(out, "body { color: red; }") {
		t.Errorf("expected style content, got %q", out)
	}
	if strings.Contains(out, "<style>") || strings.Contains(out, "</style>") {
		t.Errorf("tags should be stripped: %q", out)
	}
}

func TestHTMLBlock_TextBeforeHTMLBlock(t *testing.T) {
	out := renderOutput("text before\n<pre>\ncode\n</pre>\ntext after\n")
	if !strings.Contains(out, "text before") {
		t.Errorf("expected 'text before', got %q", out)
	}
	if !strings.Contains(out, "code") {
		t.Errorf("expected 'code', got %q", out)
	}
	if !strings.Contains(out, "text after") {
		t.Errorf("expected 'text after', got %q", out)
	}
	if strings.Contains(out, "<pre>") || strings.Contains(out, "</pre>") {
		t.Errorf("tags should be stripped: %q", out)
	}
}

func TestHTMLBlock_NoContent(t *testing.T) {
	out := renderOutput("<pre></pre>\n")
	if strings.Contains(out, "<pre>") || strings.Contains(out, "</pre>") {
		t.Errorf("tags should be stripped: %q", out)
	}
}
