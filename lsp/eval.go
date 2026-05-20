// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/token"
	"scampi.dev/scampi/linker"
	"scampi.dev/scampi/mod"
	"scampi.dev/scampi/render/template"
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
// resolve imports in user code. Errors from individual modules are
// logged and the module is skipped — the LSP must keep working even
// if a dependency is broken.
func (s *Server) loadUserModules() {
	if s.module == nil {
		return
	}
	// Use the linker's multi-file module loading so the LSP sees
	// the same merged scope (all .scampi files in the module dir)
	// that the production pipeline uses.
	userMods := linker.LoadUserModulesFromMod(s.module, s.modules)
	for _, um := range userMods {
		// Scope already set in modules by LoadUserModulesFromMod.
		// Register funcs/decls into catalog and goto-def index.
		// Walk individual files from the dep directory so each
		// decl gets registered with its real path + source bytes
		// (the merged UserModule.File loses per-file paths).
		s.registerUserModuleFiles(um.Name)
		s.log.Printf("user module %s: loaded as %q", um.Name, um.Name)
	}
}

// registerUserModuleFiles walks all .scampi files in the module's
// dependency directory (local or cached remote) and registers each
// decl with its real path + source bytes for goto-def.
func (s *Server) registerUserModuleFiles(modName string) {
	if s.module == nil {
		return
	}
	for _, dep := range s.module.Require {
		dir := depDir(s.module, &dep)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".scampi") ||
				strings.HasSuffix(e.Name(), "_test.scampi") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			f, _ := Parse(path, data)
			if f == nil || f.Module == nil || f.Module.Name.Name != modName {
				continue
			}
			s.registerModuleEntries(f, modName, path, data)
		}
	}
	// Also check the self-module directory (for non-main modules).
	selfDir := filepath.Dir(s.module.Filename)
	entries, err := os.ReadDir(selfDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".scampi") ||
			strings.HasSuffix(e.Name(), "_test.scampi") {
			continue
		}
		path := filepath.Join(selfDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f, _ := Parse(path, data)
		if f == nil || f.Module == nil || f.Module.Name.Name != modName {
			continue
		}
		s.registerModuleEntries(f, modName, path, data)
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

	// Run the full linker pipeline against the in-memory content. The
	// linker raises every diagnostic through buf; the returned error
	// is either nil, diagnostic.ErrAlreadyRaised, or a genuine outlier
	// (file-read failure, etc.) that needs a head-of-file placeholder
	// so the user sees something.
	src := newOverlaySource(filePath, data)
	buf := &diagBuf{}
	_, err := linker.Analyze(ctx, buf, filePath, src, linker.WithLenient())
	out := make([]protocol.Diagnostic, 0, len(buf.events))
	for _, ev := range buf.events {
		out = append(out, eventToLSP(ev, data))
	}
	if err != nil && !errors.Is(err, diagnostic.ErrAlreadyRaised) {
		out = append(out, protocol.Diagnostic{
			Range:    protocol.Range{},
			Severity: protocol.DiagnosticSeverityError,
			Source:   diagSourceLSP,
			Message:  err.Error(),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// diagBuf is a tiny in-process Emitter that buffers events so the LSP
// can translate them to protocol diagnostics after linker.Analyze
// returns.
type diagBuf struct {
	events []event.Event
}

func (b *diagBuf) Emit(e event.Event)          { b.events = append(b.events, e) }
func (b *diagBuf) Raise(r diagnostic.Raisable) { b.Emit(r.Diagnostic()) }

// eventToLSP renders an event.Event into LSP diagnostic shape. The
// template's Text field is a Go template string ("{{.Msg}}"); render
// it against Data so the LSP message carries the actual text rather
// than the raw template.
func eventToLSP(ev event.Event, src []byte) protocol.Diagnostic {
	tmpl := templateOf(ev)
	msg := tmpl.Text
	if rendered, ok := template.Render(tmpl.TextField()); ok {
		msg = rendered
	}
	return lspDiagFromTemplate(tmpl, src, msg, lspSeverity(ev))
}

// lspSeverity maps an event type to the matching LSP severity.
// Non-diagnostic events (Change/Progress) shouldn't reach here, but
// fall back to Information rather than misreport as Error.
func lspSeverity(ev event.Event) protocol.DiagnosticSeverity {
	switch ev.(type) {
	case event.Error:
		return protocol.DiagnosticSeverityError
	case event.Warning:
		return protocol.DiagnosticSeverityWarning
	case event.Info:
		return protocol.DiagnosticSeverityInformation
	default:
		return protocol.DiagnosticSeverityInformation
	}
}

// templateOf extracts the Template from any diagnostic-shaped event.
func templateOf(ev event.Event) event.Template {
	switch v := ev.(type) {
	case event.Error:
		return v.Template
	case event.Warning:
		return v.Template
	case event.Info:
		return v.Template
	default:
		return event.Template{}
	}
}

func lspDiagFromTemplate(
	tmpl event.Template,
	src []byte,
	msg string,
	sev protocol.DiagnosticSeverity,
) protocol.Diagnostic {
	// Render Hint/Help against tmpl.Data — the raw fields are Go
	// template strings (e.g. `{{.Hint}}`) that would otherwise leak
	// verbatim into the LSP message. Both get a labelled prefix so
	// the two sections are visually distinct in editor popups (the
	// CLI renderer uses glyph icons; LSP clients can't reliably show
	// those, so we use ASCII labels).
	if tmpl.Hint != "" {
		if rendered, ok := template.Render(tmpl.HintField()); ok {
			msg += "\n\nhint: " + rendered
		}
	}
	if tmpl.Help != "" {
		if rendered, ok := template.Render(tmpl.HelpField()); ok {
			msg += "\nhelp: " + rendered
		}
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
		Severity: sev,
		Source:   diagSourceLSP,
		Code:     string(tmpl.ID),
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
