// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"go.lsp.dev/protocol"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/lang/token"
)

// Parse parses scampi-lang source and returns the AST and any
// diagnostics. The AST is always returned when possible (even with
// errors) so callers can still do completion and hover on partial input.
func Parse(filename string, content []byte) (*ast.File, []protocol.Diagnostic) {
	l := lex.New(filename, content)
	p := parse.New(l)
	f := p.Parse()

	var diags []protocol.Diagnostic
	for _, e := range l.Errors() {
		diags = append(diags, spanDiag(content, e.Span, e.Msg))
	}
	for _, e := range p.Errors() {
		msg := e.Msg
		if e.Hint != "" {
			msg += "\nhint: " + e.Hint
		}
		diags = append(diags, spanDiag(content, e.Span, msg))
	}

	if len(diags) > 0 {
		return f, diags
	}
	return f, nil
}

// spanDiag converts a token.Span + message into an LSP diagnostic.
func spanDiag(src []byte, s token.Span, msg string) protocol.Diagnostic {
	start, end := token.ResolveSpan(src, s)
	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(max(start.Line-1, 0)),
				Character: uint32(max(start.Col-1, 0)),
			},
			End: protocol.Position{
				Line:      uint32(max(end.Line-1, 0)),
				Character: uint32(max(end.Col-1, 0)),
			},
		},
		Severity: protocol.DiagnosticSeverityError,
		Source:   "scampi",
		Message:  msg,
	}
}
