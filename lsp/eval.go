// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/lang/token"
	"scampi.dev/scampi/linker"
	"scampi.dev/scampi/mod"
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

// loadUserModules parses and type-checks user module dependencies from
// scampi.mod, adding their scopes to the module map so the checker can
// resolve imports in user code.
func (s *Server) loadUserModules() {
	if s.module == nil {
		return
	}
	for _, dep := range s.module.Require {
		dir := depDir(s.module, &dep)
		data, path := readModuleEntry(dir, lastPathSegment(dep.Path))
		if data == nil {
			s.log.Printf("user module %s: no entry point in %s", dep.Path, dir)
			continue
		}

		l := lex.New(path, data)
		p := parse.New(l)
		f := p.Parse()
		if f == nil || f.Module == nil {
			s.log.Printf("user module %s: parse failed", dep.Path)
			continue
		}

		c := check.New(s.modules)
		c.Check(f)
		modName := f.Module.Name.Name
		s.modules[modName] = c.FileScope()

		// Register funcs/decls into catalog and goto-def index.
		s.registerModuleEntries(f, modName, path, data)
		s.log.Printf("user module %s: loaded as %q", dep.Path, modName)
	}
}

// registerModuleEntries adds a user module's funcs and decls to the
// catalog (for hover/completion) and stubDefs (for goto-def).
func (s *Server) registerModuleEntries(f *ast.File, modName, filePath string, src []byte) {
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			name := modName + "." + d.Name.Name
			info := funcDeclToInfo(d, modName)
			info.Name = name
			s.catalog.funcs[name] = info
			s.stubDefs.locs[name] = stubLocation{
				path: filePath, src: src, span: d.Name.SrcSpan,
			}
		case *ast.DeclDecl:
			dn := declName(d)
			name := modName + "." + dn
			info := declDeclToInfo(d, modName)
			info.Name = name
			s.catalog.funcs[name] = info
			s.stubDefs.locs[name] = stubLocation{
				path: filePath, src: src, span: d.Name.SrcSpan,
			}
		}
	}
	// Rebuild the catalog index so new entries show up in completion.
	s.catalog.buildIndex()
}

func depDir(m *mod.Module, dep *mod.Dependency) string {
	if dep.IsLocal() {
		dir := dep.Version
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(m.Filename), dir)
		}
		return dir
	}
	return filepath.Join(mod.DefaultCacheDir(), dep.Path+"@"+dep.Version)
}

func lastPathSegment(p string) string {
	if i := len(p) - 1; i >= 0 {
		for ; i >= 0; i-- {
			if p[i] == '/' {
				return p[i+1:]
			}
		}
	}
	return p
}

// readModuleEntry finds the entry point .scampi file in a module
// directory, trying _index.scampi then <name>.scampi.
func readModuleEntry(dir, name string) ([]byte, string) {
	for _, candidate := range []string{
		filepath.Join(dir, "_index.scampi"),
		filepath.Join(dir, name+".scampi"),
	} {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data, candidate
		}
	}
	return nil, ""
}

// evaluate runs the scampi full pipeline (lex → parse → check →
// eval → attribute static checks) on the editor's current buffer and
// returns LSP diagnostics for everything it finds. The pipeline runs
// against an overlay source so the in-memory content is used instead
// of any stale on-disk version.
func (s *Server) evaluate(ctx context.Context, docURI protocol.DocumentURI, content string) []protocol.Diagnostic {
	filePath := uriToPath(docURI)
	if filePath == "" {
		return nil
	}

	data := []byte(content)

	// Fast-path parse first so we can return parse-only diagnostics
	// without doing the heavier eval pass when the file is broken.
	f, parseDiags := Parse(filePath, data)
	if len(parseDiags) > 0 {
		return parseDiags
	}
	if f == nil {
		return nil
	}

	// Run the full linker pipeline against the in-memory content.
	src := newOverlaySource(filePath, data)
	if _, err := linker.Analyze(ctx, filePath, src); err != nil {
		return analysisErrorToLSPDiagnostics(err, data)
	}
	return nil
}

// analysisErrorToLSPDiagnostics flattens the error returned by
// linker.Analyze into a slice of LSP diagnostics. linker.Analyze
// returns either a single diagnostic.Diagnostic or a
// diagnostic.Diagnostics slice (when multiple errors fire from the
// same phase); both shapes are handled.
func analysisErrorToLSPDiagnostics(err error, src []byte) []protocol.Diagnostic {
	var ds diagnostic.Diagnostics
	if errors.As(err, &ds) {
		out := make([]protocol.Diagnostic, 0, len(ds))
		for _, d := range ds {
			out = append(out, diagnosticToLSP(d, src))
		}
		return out
	}
	var d diagnostic.Diagnostic
	if errors.As(err, &d) {
		return []protocol.Diagnostic{diagnosticToLSP(d, src)}
	}
	// Non-diagnostic error (e.g. file not found): emit a placeholder
	// diagnostic at file head so the user sees something.
	return []protocol.Diagnostic{{
		Range:    protocol.Range{},
		Severity: protocol.DiagnosticSeverityError,
		Source:   "scampls",
		Message:  err.Error(),
	}}
}

// diagnosticToLSP renders a typed scampi diagnostic into the LSP
// protocol shape. Source spans on the diagnostic are used directly;
// when no span is present the diagnostic is anchored at file head so
// the user still sees the message.
func diagnosticToLSP(d diagnostic.Diagnostic, src []byte) protocol.Diagnostic {
	tmpl := d.EventTemplate()
	msg := tmpl.Text
	if e, ok := d.(error); ok {
		msg = e.Error()
	}
	if tmpl.Hint != "" {
		msg += "\n\nhint: " + tmpl.Hint
	}
	rng := protocol.Range{}
	if tmpl.Source != nil {
		// Convert 1-based line/col to 0-based LSP coordinates,
		// clamping any zero values that would underflow.
		startLine := uint32(0)
		if tmpl.Source.StartLine > 0 {
			startLine = uint32(tmpl.Source.StartLine - 1)
		}
		startCol := uint32(0)
		if tmpl.Source.StartCol > 0 {
			startCol = uint32(tmpl.Source.StartCol - 1)
		}
		endLine := startLine
		if tmpl.Source.EndLine > 0 {
			endLine = uint32(tmpl.Source.EndLine - 1)
		}
		endCol := startCol
		if tmpl.Source.EndCol > 0 {
			endCol = uint32(tmpl.Source.EndCol - 1)
		}
		rng = protocol.Range{
			Start: protocol.Position{Line: startLine, Character: startCol},
			End:   protocol.Position{Line: endLine, Character: endCol},
		}
	}
	_ = src // currently unused; reserved for future content-based span resolution
	return protocol.Diagnostic{
		Range:    rng,
		Severity: protocol.DiagnosticSeverityError,
		Source:   "scampls",
		Message:  msg,
	}
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
