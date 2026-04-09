// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"os"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/token"
	"scampi.dev/scampi/std"
)

// bootstrapModules loads the standard library stubs once so the type
// checker can resolve imports in user files.
func bootstrapModules() map[string]*check.Scope {
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		// Stubs are compiled-in; failure here is a build bug.
		panic("lsp: failed to bootstrap stdlib: " + err.Error())
	}
	return modules
}

// evaluate runs the scampi-lang lex → parse → check pipeline and
// returns LSP diagnostics.
func (s *Server) evaluate(_ context.Context, docURI protocol.DocumentURI, content string) []protocol.Diagnostic {
	filePath := uriToPath(docURI)
	if filePath == "" {
		return nil
	}

	data := []byte(content)

	// Parse.
	f, parseDiags := Parse(filePath, data)
	if len(parseDiags) > 0 {
		return parseDiags
	}
	if f == nil {
		return nil
	}

	// Type check.
	c := check.New(s.modules)
	c.Check(f)

	var diags []protocol.Diagnostic
	for _, e := range c.Errors() {
		diags = append(diags, checkerErrorToLSP(data, e))
	}
	return diags
}

func checkerErrorToLSP(src []byte, e check.Error) protocol.Diagnostic {
	return spanDiag(src, e.Span, e.Msg)
}

func uriToPath(u protocol.DocumentURI) string {
	return uri.URI(u).Filename()
}

// spanToRange converts a token.Span to an LSP range, resolving byte
// offsets to line/column via the source bytes.
func tokenSpanToRange(src []byte, s token.Span) protocol.Range {
	start, end := token.ResolveSpan(src, s)
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(max(start.Line-1, 0)),
			Character: uint32(max(start.Col-1, 0)),
		},
		End: protocol.Position{
			Line:      uint32(max(end.Line-1, 0)),
			Character: uint32(max(end.Col-1, 0)),
		},
	}
}

// diagnoseFile reads and diagnoses a single file on disk.
func (s *Server) diagnoseFile(ctx context.Context, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	docURI := protocol.DocumentURI(uri.File(path))
	diags := s.evaluate(ctx, docURI, string(data))
	if diags == nil {
		diags = []protocol.Diagnostic{}
	}
	s.log.Printf("workspace diag: %s → %d", path, len(diags))
	_ = s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         docURI,
		Diagnostics: diags,
	})
}
