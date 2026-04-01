// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"scampi.dev/scampi/diagnostic"
	rtmpl "scampi.dev/scampi/render/template"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/star"
	"scampi.dev/scampi/star/testkit"
)

// Evaluate runs the full Starlark evaluation pipeline and returns LSP
// diagnostics. This catches everything the engine would catch: unknown
// kwargs, missing required fields, type errors, invalid enum values, etc.
func Evaluate(ctx context.Context, docURI protocol.DocumentURI, content string) []protocol.Diagnostic {
	filePath := uriToPath(docURI)
	if filePath == "" {
		return nil
	}

	dir := filepath.Dir(filePath)
	base := source.LocalPosixSource{}
	src := source.WithRoot(dir, &overlaySource{
		base:    base,
		path:    filePath,
		content: []byte(content),
	})
	store := diagnostic.NewSourceStore()

	var opts []star.EvalOption
	if strings.HasSuffix(filePath, "_test.scampi") {
		opts = append(opts, star.WithTestBuiltins(testkit.NewCollector()))
	}

	_, err := star.Eval(ctx, filePath, store, src, opts...)
	if err == nil {
		return nil
	}

	return evalErrors(err)
}

func evalErrors(err error) []protocol.Diagnostic {
	// Multi-diagnostic errors (multiple issues collected).
	var multi diagnostic.MultiDiagnostic
	if errors.As(err, &multi) {
		var diags []protocol.Diagnostic
		for _, d := range multi.Diagnostics() {
			diags = append(diags, diagnosticToLSP(d))
		}
		return diags
	}

	// Single diagnostic error.
	var diag diagnostic.Diagnostic
	if errors.As(err, &diag) {
		return []protocol.Diagnostic{diagnosticToLSP(diag)}
	}

	// Fallback: could be a syntax error from the Starlark parser.
	return syntaxErrors(err)
}

func diagnosticToLSP(d diagnostic.Diagnostic) protocol.Diagnostic {
	tmpl := d.EventTemplate()

	severity := protocol.DiagnosticSeverityError
	switch d.Severity() {
	case signal.Warning:
		severity = protocol.DiagnosticSeverityWarning
	case signal.Info, signal.Notice:
		severity = protocol.DiagnosticSeverityInformation
	case signal.Debug:
		severity = protocol.DiagnosticSeverityHint
	}

	msg, _ := rtmpl.Render(tmpl.TextField())
	if hint, ok := rtmpl.Render(tmpl.HintField()); ok {
		msg += "\nhint: " + hint
	}

	r := protocol.Range{}
	if tmpl.Source != nil {
		r = spanToRange(*tmpl.Source)
	}

	return protocol.Diagnostic{
		Range:    r,
		Severity: severity,
		Source:   "scampi",
		Message:  msg,
	}
}

func spanToRange(s spec.SourceSpan) protocol.Range {
	startLine := uint32(0)
	if s.StartLine > 0 {
		startLine = uint32(s.StartLine - 1)
	}
	startCol := uint32(0)
	if s.StartCol > 0 {
		startCol = uint32(s.StartCol - 1)
	}
	endLine := startLine
	if s.EndLine > 0 {
		endLine = uint32(s.EndLine - 1)
	}
	endCol := startCol
	if s.EndCol > 0 {
		endCol = uint32(s.EndCol - 1)
	}
	return protocol.Range{
		Start: protocol.Position{Line: startLine, Character: startCol},
		End:   protocol.Position{Line: endLine, Character: endCol},
	}
}

func uriToPath(u protocol.DocumentURI) string {
	return uri.URI(u).Filename()
}

// overlaySource wraps a real source.Source but returns in-memory content
// for a single file (the open document). Everything else passes through.
type overlaySource struct {
	base    source.Source
	path    string
	content []byte
}

func (o *overlaySource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	abs, _ := filepath.Abs(path)
	if abs == o.path {
		return o.content, nil
	}
	return o.base.ReadFile(ctx, path)
}

func (o *overlaySource) WriteFile(ctx context.Context, path string, data []byte) error {
	return o.base.WriteFile(ctx, path, data)
}

func (o *overlaySource) EnsureDir(ctx context.Context, path string) error {
	return o.base.EnsureDir(ctx, path)
}

func (o *overlaySource) Stat(ctx context.Context, path string) (source.FileMeta, error) {
	return o.base.Stat(ctx, path)
}

func (o *overlaySource) LookupEnv(key string) (string, bool) {
	return o.base.LookupEnv(key)
}

func (o *overlaySource) LookupSecret(key string) (string, bool, error) {
	return o.base.LookupSecret(key)
}
