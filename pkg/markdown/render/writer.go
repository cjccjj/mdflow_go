package render

import (
	"github.com/cjccjj/mdflow/pkg/markdown/event"
)

// InlineRenderer renders Markdown used inside a table cell. It is injected so the
// production renderer does not depend on the parser package.
type InlineRenderer func(text string) string

type Writer struct {
	aw                 *AnsiWriter
	theme              Theme
	live               bool
	tableActive        bool
	inBlockquote       bool
	inListItem         bool
	indentNextText     bool
	inBold             int
	inItalic           int
	inStrikethrough    int
	activeHeaderSuffix string
	tableHeader        []string
	tableRows          [][]string
	tableWidths        []int
	tableAligns        []int
	tableLines         int
	termWidth          int
	tableRepaintCount  int
	inlineRenderer     InlineRenderer
}

func NewWriter(aw *AnsiWriter, theme Theme) *Writer {
	return &Writer{aw: aw, theme: theme}
}

func (w *Writer) SetInlineRenderer(renderer InlineRenderer) {
	w.inlineRenderer = renderer
}

func (w *Writer) renderInline(text string) string {
	if w.inlineRenderer != nil {
		return w.inlineRenderer(text)
	}
	return RenderInline(text, w.theme)
}

func (w *Writer) SetLive(v bool) {
	w.live = v
}

func (w *Writer) SetTermWidth(width int) {
	w.termWidth = width
}

func (w *Writer) Handle(e event.Event) error {
	switch e.Type {
	case event.TextEvent:
		if w.indentNextText {
			w.indentNextText = false
			if _, err := w.aw.WriteString("  "); err != nil {
				return err
			}
		}
		_, err := w.aw.WriteString(e.Value)
		return err

	case event.NewlineEvent:
		if _, err := w.aw.WriteString("\n"); err != nil {
			return err
		}
		if w.inListItem {
			w.indentNextText = true
		}
		if w.inBlockquote {
			_, err := w.aw.WriteStyled("│ ", w.theme.Blockquote)
			return err
		}
		return nil

	case event.BlockquoteStartEvent:
		w.inBlockquote = true
		_, err := w.aw.WriteStyled("│ ", w.theme.Blockquote)
		return err

	case event.BlockquoteEndEvent:
		w.inBlockquote = false
		return nil

	case event.HeaderStartEvent:
		style := w.theme.H1
		prefix := ""
		switch e.Level {
		case 2:
			style = w.theme.H2
			prefix = "## "
		case 3:
			style = w.theme.H3
			prefix = "### "
		case 4:
			style = w.theme.H4
			prefix = "#### "
		case 5:
			style = w.theme.H5
			prefix = "##### "
		case 6:
			style = w.theme.H6
			prefix = "###### "
		}
		w.activeHeaderSuffix = style.Suffix
		_, err := w.aw.WriteString(style.Prefix + prefix)
		return err

	case event.HeaderEndEvent:
		if w.activeHeaderSuffix != "" {
			_, err := w.aw.WriteString(w.activeHeaderSuffix)
			w.activeHeaderSuffix = ""
			return err
		}
		_, err := w.aw.WriteString(w.theme.H1.Suffix)
		return err

	case event.BoldStartEvent:
		return w.enterEmphasis("bold")

	case event.BoldEndEvent:
		return w.exitEmphasis("bold")

	case event.ItalicStartEvent:
		return w.enterEmphasis("italic")

	case event.ItalicEndEvent:
		return w.exitEmphasis("italic")

	case event.StrikethroughStartEvent:
		return w.enterEmphasis("strikethrough")

	case event.StrikethroughEndEvent:
		return w.exitEmphasis("strikethrough")

	case event.InlineCodeStartEvent:
		_, err := w.aw.WriteString(w.theme.InlineCode.Prefix)
		return err

	case event.InlineCodeEndEvent:
		_, err := w.aw.WriteString(w.theme.InlineCode.Suffix)
		return err

	case event.CodeBlockStartEvent:
		_, err := w.aw.WriteString(w.theme.CodeBlock.Prefix)
		return err

	case event.CodeBlockEndEvent:
		_, err := w.aw.WriteString(w.theme.CodeBlock.Suffix)
		return err

	case event.CodeBlockLangEvent:
		_, err := w.aw.WriteString(w.theme.CodeBlockLang.Prefix + e.Value + w.theme.CodeBlockLang.Suffix)
		return err

	case event.HorizontalRuleEvent:
		if _, err := w.aw.WriteString(w.theme.HorizontalRule.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString("────────────────"); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.HorizontalRule.Suffix); err != nil {
			return err
		}
		_, err := w.aw.WriteString("\n")
		return err

	case event.BulletItemEvent:
		w.inListItem = true
		w.indentNextText = false
		if e.Value != "" {
			_, err := w.aw.WriteString(e.Value)
			return err
		}
		_, err := w.aw.WriteString("• ")
		return err

	case event.TableStartEvent:
		return w.handleTableStart(e)

	case event.TableRowEvent:
		return w.handleTableRow(e)

	case event.TableEndEvent:
		return w.handleTableEnd()

	case event.LinkEvent:
		if _, err := w.aw.WriteString(w.theme.LinkText.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.Value); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkText.Suffix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(" ("); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkURL.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.URL); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkURL.Suffix); err != nil {
			return err
		}
		if e.Title != "" {
			if _, err := w.aw.WriteString(" \""); err != nil {
				return err
			}
			if _, err := w.aw.WriteString(e.Title); err != nil {
				return err
			}
			if _, err := w.aw.WriteString("\""); err != nil {
				return err
			}
		}
		_, err := w.aw.WriteString(")")
		return err

	case event.ImageEvent:
		if _, err := w.aw.WriteString(w.theme.ImageLabel.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString("[IMG: "); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.ImageLabel.Suffix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkText.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.Value); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkText.Suffix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(" ("); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkURL.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.URL); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkURL.Suffix); err != nil {
			return err
		}
		if e.Title != "" {
			if _, err := w.aw.WriteString(" \""); err != nil {
				return err
			}
			if _, err := w.aw.WriteString(e.Title); err != nil {
				return err
			}
			if _, err := w.aw.WriteString("\""); err != nil {
				return err
			}
		}
		_, err := w.aw.WriteString(")")
		return err

	case event.LinkRefDefEvent:
		if _, err := w.aw.WriteString("\033[2m[Link Def: "); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.Value); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(" \u2192 "); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.URL); err != nil {
			return err
		}
		if _, err := w.aw.WriteString("]\033[0m"); err != nil {
			return err
		}
		return nil

	case event.LinkRefEvent:
		if _, err := w.aw.WriteString(w.theme.LinkText.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.Value); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkText.Suffix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(" [\u2192 "); err != nil {
			return err
		}
		label := e.URL
		if label == "" {
			label = "ref"
		}
		if _, err := w.aw.WriteString(label); err != nil {
			return err
		}
		_, err := w.aw.WriteString("]")
		return err

	case event.ImageRefEvent:
		if _, err := w.aw.WriteString(w.theme.ImageLabel.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString("[IMG: "); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.ImageLabel.Suffix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkText.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.Value); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(w.theme.LinkText.Suffix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(" [\u2192 "); err != nil {
			return err
		}
		label := e.URL
		if label == "" {
			label = "ref"
		}
		if _, err := w.aw.WriteString(label); err != nil {
			return err
		}
		_, err := w.aw.WriteString("]")
		return err

	case event.AutolinkURLEvent:
		if _, err := w.aw.WriteString(w.theme.LinkURL.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.Value); err != nil {
			return err
		}
		_, err := w.aw.WriteString(w.theme.LinkURL.Suffix)
		return err

	case event.AutolinkEmailEvent:
		if _, err := w.aw.WriteString(w.theme.LinkText.Prefix); err != nil {
			return err
		}
		if _, err := w.aw.WriteString(e.Value); err != nil {
			return err
		}
		_, err := w.aw.WriteString(w.theme.LinkText.Suffix)
		return err

	default:
		return nil
	}
}

func (w *Writer) emphasisActive() bool {
	return w.inBold > 0 || w.inItalic > 0 || w.inStrikethrough > 0
}

func (w *Writer) emphasisSGR() string {
	codes := make([]string, 0, 3)
	if w.inBold > 0 {
		codes = append(codes, "1")
	}
	if w.inItalic > 0 {
		codes = append(codes, "3")
	}
	if w.inStrikethrough > 0 {
		codes = append(codes, "9")
	}
	if len(codes) == 0 {
		return ""
	}
	sgr := "\033["
	for i, c := range codes {
		if i > 0 {
			sgr += ";"
		}
		sgr += c
	}
	return sgr + "m"
}

func (w *Writer) enterEmphasis(kind string) error {
	hadActive := w.emphasisActive()

	switch kind {
	case "bold":
		w.inBold++
	case "italic":
		w.inItalic++
	case "strikethrough":
		w.inStrikethrough++
	}

	if hadActive {
		if _, err := w.aw.WriteString("\033[0m"); err != nil {
			return err
		}
		sgr := w.emphasisSGR()
		if sgr != "" {
			_, err := w.aw.WriteString(sgr)
			return err
		}
		return nil
	}

	themePrefix := ""
	switch kind {
	case "bold":
		themePrefix = w.theme.Bold.Prefix
	case "italic":
		themePrefix = w.theme.Italic.Prefix
	case "strikethrough":
		themePrefix = w.theme.Strikethrough.Prefix
	}
	_, err := w.aw.WriteString(themePrefix)
	return err
}

func (w *Writer) exitEmphasis(kind string) error {
	switch kind {
	case "bold":
		w.inBold--
	case "italic":
		w.inItalic--
	case "strikethrough":
		w.inStrikethrough--
	}

	stillActive := w.emphasisActive()

	if stillActive {
		if _, err := w.aw.WriteString("\033[0m"); err != nil {
			return err
		}
		sgr := w.emphasisSGR()
		if sgr != "" {
			_, err := w.aw.WriteString(sgr)
			return err
		}
		return nil
	}

	themeSuffix := ""
	switch kind {
	case "bold":
		themeSuffix = w.theme.Bold.Suffix
	case "italic":
		themeSuffix = w.theme.Italic.Suffix
	case "strikethrough":
		themeSuffix = w.theme.Strikethrough.Suffix
	}
	_, err := w.aw.WriteString(themeSuffix)
	return err
}

func (w *Writer) ResetStyles() error {
	w.inBlockquote = false
	w.inListItem = false
	w.indentNextText = false
	w.inBold = 0
	w.inItalic = 0
	w.inStrikethrough = 0
	w.activeHeaderSuffix = ""
	_, err := w.aw.WriteString("\033[0m")
	return err
}
