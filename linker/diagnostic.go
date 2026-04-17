// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

type langDiagData struct {
	Msg  string
	Hint string
}

// langDiagnostic wraps a plain lang pipeline error as a
// diagnostic.Diagnostic so the engine can render it properly.
type langDiagnostic struct {
	diagnostic.FatalError
	code errs.Code
	msg  string
	hint string
	src  *spec.SourceSpan
}

func (d *langDiagnostic) Error() string { return d.msg }

func (d *langDiagnostic) EventTemplate() event.Template {
	t := event.Template{
		ID:     d.code,
		Text:   "{{.Msg}}",
		Source: d.src,
		Data:   langDiagData{Msg: d.msg, Hint: d.hint},
	}
	if d.hint != "" {
		t.Hint = "{{.Hint}}"
	}
	return t
}

// wrapLangError converts a plain error from the lang pipeline into
// a diagnostic.Diagnostic the engine can render.
// wrapLangErrors wraps multiple lang errors as a diagnostic.Diagnostics
// slice so the engine emits all of them, not just the first.
func wrapLangErrors[T error](errs []T, cfgPath string, source []byte) error {
	if len(errs) == 1 {
		return wrapLangError(errs[0], cfgPath, source)
	}
	var diags diagnostic.Diagnostics
	for _, e := range errs {
		diags = append(diags, wrapLangError(e, cfgPath, source).(*langDiagnostic))
	}
	return diags
}

func wrapLangError(err error, cfgPath string, source []byte) error {
	span := &spec.SourceSpan{Filename: cfgPath}

	var hint string

	// Try to extract source span and hint from typed lang errors.
	type spanned interface {
		GetSpan() (start, end uint32)
	}
	if se, ok := err.(spanned); ok {
		start, end := se.GetSpan()
		if start > 0 && len(source) > 0 {
			sLine, sCol := offsetToLineCol(source, int(start))
			eLine, eCol := offsetToLineCol(source, int(end))
			span.StartLine = sLine
			span.StartCol = sCol
			span.EndLine = eLine
			span.EndCol = eCol
		}
	}
	type hinted interface {
		GetHint() string
	}
	if h, ok := err.(hinted); ok {
		hint = h.GetHint()
	}

	code := errs.Code("lang.Error")
	type coded interface {
		GetCode() errs.Code
	}
	if ce, ok := err.(coded); ok && ce.GetCode() != "" {
		code = ce.GetCode()
	}

	return &langDiagnostic{
		code: code,
		msg:  err.Error(),
		hint: hint,
		src:  span,
	}
}

func offsetToLineCol(src []byte, offset int) (line, col int) {
	line = 1
	col = 1
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}
