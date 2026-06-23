// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
)

type langDiagData struct {
	Msg  string
	Hint string
}

// langDiagnostic wraps a plain lang pipeline error as a Raisable so
// the engine can render it properly.
type langDiagnostic struct {
	code errs.Code
	msg  string
	hint string
	src  *spec.SourceSpan
}

func (d *langDiagnostic) Error() string { return d.msg }

func (d *langDiagnostic) Diagnostic() event.Event {
	t := event.Template{
		ID:     d.code,
		Text:   "{{.Msg}}",
		Source: d.src,
		Data:   langDiagData{Msg: d.msg, Hint: d.hint},
	}
	if d.hint != "" {
		t.Hint = "{{.Hint}}"
	}
	return event.Error{Impact: event.ImpactAbort, Template: t}
}

// raiseLangErrors raises each error from the lang pipeline through em
// as an individual diagnostic. Returns true if any were raised.
func raiseLangErrors[T error](em diagnostic.Emitter, errs []T, cfgPath string, source []byte) bool {
	for _, e := range errs {
		em.Raise(toLangDiagnostic(e, cfgPath, source))
	}
	return len(errs) > 0
}

// raiseBrokenSiblings emits one diagnostic per broken sibling file so
// the user sees why symbols from those files are missing.
func raiseBrokenSiblings(em diagnostic.Emitter, broken []brokenSibling) {
	seen := map[string]bool{}
	for _, b := range broken {
		if seen[b.path] {
			continue
		}
		seen[b.path] = true
		em.Raise(&langDiagnostic{
			code: CodeBrokenSibling,
			msg:  "sibling file " + b.path + " has errors and was skipped",
			hint: b.firstErr,
			src:  &spec.SourceSpan{Filename: b.path},
		})
	}
}

func toLangDiagnostic(err error, cfgPath string, source []byte) *langDiagnostic {
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
