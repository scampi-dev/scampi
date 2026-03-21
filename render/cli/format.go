// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/render/ansi"
	"scampi.dev/scampi/render/template"
	"scampi.dev/scampi/spec"
)

type sourceLine struct {
	filename string
	line     int
	startCol int
	endCol   int
	text     string
	ok       bool
}

type formatter struct {
	glyphs   glyphSet
	useColor bool
	store    *diagnostic.SourceStore
}

func newFormatter(glyphs glyphSet, useColor bool, store *diagnostic.SourceStore) *formatter {
	return &formatter{glyphs: glyphs, useColor: useColor, store: store}
}

func (f *formatter) fmtMsg(color ansi.ANSI, msg string) string {
	var buf strings.Builder
	f.fmtMsgTo(&buf, color, msg)
	return buf.String()
}

func (f *formatter) fmtfMsg(color ansi.ANSI, format string, args ...any) string {
	var buf strings.Builder
	f.fmtfMsgTo(&buf, color, format, args...)
	return buf.String()
}

func (f *formatter) fmtMsgTo(w io.Writer, color ansi.ANSI, msg string) {
	if !f.useColor {
		_, _ = fmt.Fprint(w, msg)
		return
	}
	_, _ = fmt.Fprint(w, color)
	_, _ = fmt.Fprint(w, msg)
	_, _ = fmt.Fprint(w, ansi.Reset)
}

func (f *formatter) fmtfMsgTo(w io.Writer, color ansi.ANSI, format string, args ...any) {
	if !f.useColor {
		_, _ = fmt.Fprintf(w, format, args...)
		return
	}
	_, _ = fmt.Fprint(w, color)
	_, _ = fmt.Fprintf(w, format, args...)
	_, _ = fmt.Fprint(w, ansi.Reset)
}

func (f *formatter) fmtTemplate(
	tmpl event.Template,
	prefix, msg, glyph string,
	txtCol, helpCol ansi.ANSI,
) []string {
	var buf strings.Builder

	if text, ok := template.Render(tmpl.TextField()); ok {
		f.fmtfMsgTo(&buf, txtCol, "[%s]%s %s%s", prefix, glyphR(glyph), text, msg)
	}

	if snippet, ok := f.renderSnippet(tmpl.Source); ok {
		buf.WriteString("\n")
		buf.WriteString(snippet)
	}

	if hint, ok := template.Render(tmpl.HintField()); ok {
		hint = strings.TrimSpace(hint)
		if hint != "" {
			buf.WriteString("\n    ")
			f.fmtfMsgTo(&buf, helpCol, "%s hint:", glyphL(f.glyphs.hint))
			for l := range strings.SplitSeq(hint, "\n") {
				buf.WriteString("\n    ")
				f.fmtfMsgTo(&buf, helpCol, "     %s", l)
			}
		}
	}

	if help, ok := template.Render(tmpl.HelpField()); ok {
		help = strings.TrimSpace(help)
		if help != "" {
			buf.WriteString("\n    ")
			f.fmtfMsgTo(&buf, helpCol, "%s help:", glyphL(f.glyphs.help))
			for l := range strings.SplitSeq(help, "\n") {
				buf.WriteString("\n    ")
				f.fmtfMsgTo(&buf, helpCol, "     %s", l)
			}
		}
	}

	return strings.Split(strings.TrimSpace(buf.String()), "\n")
}

func (f *formatter) renderSnippet(src *spec.SourceSpan) (string, bool) {
	if src == nil || f.store == nil || *src == (spec.SourceSpan{}) {
		return "", false
	}
	v := f.loadSourceLine(src)
	var b strings.Builder
	f.renderSourceHeader(&b, v)
	b.WriteString("\n")
	f.renderSourceBody(&b, v)
	return b.String(), true
}

func (f *formatter) loadSourceLine(src *spec.SourceSpan) sourceLine {
	text, ok := f.store.Line(src.Filename, src.StartLine)
	endCol := src.EndCol
	if src.StartLine < src.EndLine {
		endCol = len(text) + 1
	}
	return sourceLine{
		filename: src.Filename,
		line:     src.StartLine,
		startCol: src.StartCol,
		endCol:   endCol,
		text:     text,
		ok:       ok,
	}
}

func (f *formatter) renderSourceHeader(w io.Writer, v sourceLine) {
	_, _ = fmt.Fprintf(w, "  --> %s:%d:%d", v.filename, v.line, v.startCol)
}

func (f *formatter) renderSourceBody(w io.Writer, v sourceLine) {
	gutter := f.fmtfMsg(colSourceGutter, "|")

	if !v.ok {
		_, _ = fmt.Fprintf(w, "   %s <source unavailable>", gutter)
		return
	}

	lineNo := strconv.Itoa(v.line)
	pad := strings.Repeat(" ", len(lineNo))

	_, _ = fmt.Fprintf(w, "  %s %s\n", pad, gutter)
	_, _ = fmt.Fprintf(w, "  %s%s%s %s %s\n", colSourceGutter, lineNo, ansi.Reset, gutter, v.text)

	if v.startCol > 0 {
		_, _ = fmt.Fprintf(w, "  %s %s %s", pad, gutter, caretPadding(v.text, v.startCol))
		f.fmtMsgTo(w, colSourceCaret, underlineRange(v.startCol, v.endCol))
	}
}

func caretPadding(line string, col int) string {
	if col <= 1 {
		return ""
	}
	var b strings.Builder
	i := 1
	for _, r := range line {
		if i >= col {
			break
		}
		switch r {
		case '\t':
			b.WriteRune('\t')
		default:
			b.WriteRune(' ')
		}
		i++
	}
	return b.String()
}

func underlineRange(start, end int) string {
	if end <= start {
		return "^"
	}
	return strings.Repeat("~", end-start)
}
