// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
)

func (s *Server) Completion(
	_ context.Context,
	params *protocol.CompletionParams,
) (*protocol.CompletionList, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		s.log.Printf("completion: no doc for %s", params.TextDocument.URI)
		return nil, nil
	}

	cur := AnalyzeCursor(doc.Content, params.Position.Line, params.Position.Character)
	s.log.Printf(
		"completion: line=%d col=%d word=%q inCall=%v inList=%v func=%q",
		params.Position.Line,
		params.Position.Character,
		cur.WordUnderCursor,
		cur.InCall,
		cur.InList,
		cur.FuncName,
	)

	var items []protocol.CompletionItem

	switch {
	case cur.InCall && cur.InString:
		// Check whether the active parameter carries an attribute
		// that has a registered LSP completion provider (e.g.
		// `@secretkey` → completeSecretKeys). Falls through to enum
		// completion when no provider matches.
		items = s.dispatchAttributeProvider(params.TextDocument.URI, cur)
		if items == nil && cur.ActiveKwarg != "" {
			items = s.completeEnumValues(params.TextDocument.URI, cur)
		}
	case cur.InCall && cur.ActiveKwarg != "":
		items = s.completeKwargValue(params.TextDocument.URI, cur)
	case cur.InList:
		items = s.completeTopLevel(params.TextDocument.URI, cur.WordUnderCursor)
	case cur.InCall:
		items = s.completeKwargs(params.TextDocument.URI, cur)
	case isDotPrefix(cur.WordUnderCursor):
		items = s.completeModule(params.TextDocument.URI, cur.WordUnderCursor)
	default:
		items = s.completeTopLevel(params.TextDocument.URI, cur.WordUnderCursor)
	}

	s.log.Printf("completion: returning %d items", len(items))
	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}

// completeTopLevel offers all top-level names: stdlib catalog
// members, module namespaces, and user-defined funcs/decls/types/
// lets from the current document.
func (s *Server) completeTopLevel(
	docURI protocol.DocumentURI,
	prefix string,
) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	for _, name := range s.catalog.Names() {
		// Skip dotted names at top level — offer the module name instead.
		if strings.Contains(name, ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}

		f, _ := s.catalog.Lookup(name)
		kind := protocol.CompletionItemKindFunction
		items = append(items, protocol.CompletionItem{
			Label:         name,
			Kind:          kind,
			Detail:        f.Summary,
			InsertText:    name + "(",
			Documentation: f.Summary,
		})
	}

	// Offer module names.
	for _, mod := range s.catalog.Modules() {
		if prefix != "" && !strings.HasPrefix(mod, prefix) {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label:      mod,
			Kind:       protocol.CompletionItemKindModule,
			Detail:     mod + " namespace",
			InsertText: mod + ".",
		})
	}

	// Offer user-defined names from the current document — funcs,
	// decls, types, top-level lets, and any nested lets/params the
	// checker captured in its flat fallback map.
	items = append(items, s.userDeclItems(docURI, prefix)...)

	return items
}

// userDeclItems returns completion items for every user-defined
// name reachable from the current document. Pulls from the file
// scope (top-level decls + imports) and the checker's flat
// AllBindings (nested let/param bindings).
func (s *Server) userDeclItems(
	docURI protocol.DocumentURI,
	prefix string,
) []protocol.CompletionItem {
	doc, ok := s.docs.Get(docURI)
	if !ok {
		return nil
	}
	filePath := uriToPath(docURI)
	if filePath == "" {
		return nil
	}
	c := tolerantCheck(filePath, []byte(doc.Content), s.modules)
	if c == nil {
		return nil
	}
	scope := c.FileScope()
	if scope == nil {
		return nil
	}

	// Deduplicate by name; file scope wins over allBindings.
	seen := make(map[string]bool)
	var items []protocol.CompletionItem

	add := func(name string, sym *check.Symbol) {
		if seen[name] {
			return
		}
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			return
		}
		// Skip imports — they're already offered as module names
		// via the catalog path, no need to double-count.
		if sym.Kind == check.SymImport {
			return
		}
		seen[name] = true
		items = append(items, userDeclItem(name, sym))
	}

	for name, sym := range scope.Symbols() {
		add(name, sym)
	}
	for name, sym := range c.AllBindings() {
		add(name, sym)
	}
	return items
}

// userDeclItem builds a completion item for a user-defined name.
// Picks an LSP item kind based on the symbol kind and produces a
// sensible insertText (function names get a trailing `(`, struct
// types get a `{`, etc.).
func userDeclItem(name string, sym *check.Symbol) protocol.CompletionItem {
	kind := protocol.CompletionItemKindVariable
	insert := name
	switch sym.Kind {
	case check.SymFunc:
		kind = protocol.CompletionItemKindFunction
		insert = name + "("
	case check.SymDecl:
		kind = protocol.CompletionItemKindFunction
		insert = name + " {"
	case check.SymType:
		kind = protocol.CompletionItemKindStruct
		insert = name + " {"
	case check.SymEnum:
		kind = protocol.CompletionItemKindEnum
	case check.SymLet, check.SymParam:
		kind = protocol.CompletionItemKindVariable
	}
	detail := ""
	if sym.Type != nil {
		detail = sym.Type.String()
	}
	return protocol.CompletionItem{
		Label:      name,
		Kind:       kind,
		Detail:     detail,
		InsertText: insert,
	}
}

// completeModule offers members of a dotted prefix. The lhs of the
// dot is interpreted in priority order:
//
//  1. **Module name** — if `mod` matches a known module in the
//     catalog (e.g. `posix.`), offer that module's members.
//  2. **UFCS receiver** — if `mod` is a let-bound variable in the
//     current document, offer free functions whose first parameter
//     accepts the variable's type. This is what makes
//     `t.assert_file_exists("/p")` discoverable when `t` is a
//     `test.Target` and the function is declared as
//     `func assert_file_exists(t Target, p string)`.
func (s *Server) completeModule(docURI protocol.DocumentURI, word string) []protocol.CompletionItem {
	dot := strings.LastIndexByte(word, '.')
	if dot < 0 {
		return nil
	}
	mod := word[:dot]
	prefix := word[dot+1:]

	members := s.catalog.ModuleMembers(mod)
	var items []protocol.CompletionItem
	for _, member := range members {
		if prefix != "" && !strings.HasPrefix(member, prefix) {
			continue
		}
		fullName := mod + "." + member
		f, ok := s.catalog.Lookup(fullName)
		if !ok {
			continue
		}
		var insertText string
		if f.IsStep {
			insertText = member + " {"
		} else {
			insertText = member + "("
		}
		items = append(items, protocol.CompletionItem{
			Label:         member,
			Kind:          protocol.CompletionItemKindFunction,
			Detail:        f.Summary,
			InsertText:    insertText,
			Documentation: f.Summary,
		})
	}
	if len(items) > 0 {
		return items
	}

	// Module-name path didn't match anything. Try UFCS — `mod` may
	// be a value reference whose type has free functions in scope
	// that accept it as a first arg.
	return s.completeUFCS(docURI, mod, prefix)
}

// completeUFCS offers UFCS-eligible free functions for a `varname.`
// callsite. It runs the lang pipeline (parse + check) on the current
// document tolerantly, looks up `varName` in the file scope first
// and falls back to the checker's flat allBindings map if the
// receiver lives inside a function body or other nested scope.
// Then it walks file-scope functions and module exports, returning
// those whose first parameter accepts the receiver's type.
//
// Fails open: any missing parse/scope/binding returns nil — the
// user is mid-edit and the LSP just won't show UFCS completions for
// that moment.
func (s *Server) completeUFCS(
	docURI protocol.DocumentURI,
	varName, prefix string,
) []protocol.CompletionItem {
	doc, ok := s.docs.Get(docURI)
	if !ok {
		return nil
	}
	filePath := uriToPath(docURI)
	if filePath == "" {
		return nil
	}
	c := tolerantCheck(filePath, []byte(doc.Content), s.modules)
	if c == nil {
		return nil
	}
	scope := c.FileScope()
	if scope == nil {
		return nil
	}

	// File scope first (top-level lets). Fall back to the flat
	// allBindings map for nested bindings (lets inside function
	// bodies, parameters, for-loop vars).
	sym := scope.Lookup(varName)
	if sym == nil || sym.Type == nil {
		if alt, ok := c.AllBindings()[varName]; ok && alt.Type != nil {
			sym = alt
		}
	}
	if sym == nil || sym.Type == nil {
		return nil
	}
	recvType := sym.Type

	var items []protocol.CompletionItem

	// File-scope functions (top-level decls in this document).
	for name, fnSym := range scope.Symbols() {
		if !ufcsAccepts(fnSym, recvType) {
			continue
		}
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		items = append(items, ufcsItem(name, fnSym))
	}

	// Functions from imported modules.
	for _, modScope := range s.modules {
		for name, fnSym := range modScope.Symbols() {
			if !ufcsAccepts(fnSym, recvType) {
				continue
			}
			if prefix != "" && !strings.HasPrefix(name, prefix) {
				continue
			}
			items = append(items, ufcsItem(name, fnSym))
		}
	}

	return items
}

// ufcsAccepts reports whether sym is a function whose first param
// accepts the receiver type — i.e. whether `recv.<sym.Name>(...)`
// would resolve to `<sym.Name>(recv, ...)` under UFCS rules.
func ufcsAccepts(sym *check.Symbol, recv check.Type) bool {
	if sym == nil || sym.Kind != check.SymFunc {
		return false
	}
	ft, ok := sym.Type.(*check.FuncType)
	if !ok || len(ft.Params) == 0 {
		return false
	}
	return check.IsAssignableTo(recv, ft.Params[0].Type)
}

// ufcsItem builds a completion item for a UFCS-eligible function.
// Returns the bare name (no module qualifier) since the user's
// receiver expression already supplies the prefix.
func ufcsItem(name string, sym *check.Symbol) protocol.CompletionItem {
	return protocol.CompletionItem{
		Label:      name,
		Kind:       protocol.CompletionItemKindMethod,
		Detail:     "(ufcs) " + sym.Type.String(),
		InsertText: name + "(",
	}
}

// tolerantCheck runs lex+parse+check on a buffer and returns the
// Checker even when there are parse or check errors. The
// forward-declaration pass populates the file scope before any
// expression-level walking, and the checker's flat AllBindings map
// captures every binding seen during the walk regardless of which
// scope it lived in. Both are usable from this point forward by
// the LSP for completion and hover.
//
// Returns nil only when the parser produces no file at all.
func tolerantCheck(
	path string,
	data []byte,
	modules map[string]*check.Scope,
) *check.Checker {
	l := lex.New(path, data)
	p := parse.New(l)
	f := p.Parse()
	if f == nil {
		return nil
	}
	c := check.New(modules)
	c.Check(f)
	return c
}

// completeKwargs offers keyword arguments for the function being called.
func (s *Server) completeKwargs(docURI protocol.DocumentURI, cur CursorContext) []protocol.CompletionItem {
	f, ok := s.lookupFunc(docURI, cur.FuncName)
	if !ok {
		return nil
	}

	present := make(map[string]bool, len(cur.PresentKwargs))
	for _, k := range cur.PresentKwargs {
		present[k] = true
	}

	var items []protocol.CompletionItem
	for _, p := range f.Params {
		if present[p.Name] {
			continue
		}

		detail := p.Type
		if p.Required {
			detail += " (required)"
		}

		doc := p.Desc
		if p.Default != "" {
			doc += fmt.Sprintf("\n\nDefault: %s", p.Default)
		}
		if len(p.Examples) > 0 {
			doc += fmt.Sprintf("\n\nExamples: %s", strings.Join(p.Examples, ", "))
		}

		items = append(items, protocol.CompletionItem{
			Label:         p.Name,
			Kind:          protocol.CompletionItemKindProperty,
			Detail:        detail,
			InsertText:    p.Name + " = ",
			Documentation: doc,
		})
	}
	return items
}

// Attribute-driven completion providers
// -----------------------------------------------------------------------------

// AttributeCompletionProvider produces completion items for a
// parameter that carries a particular attribute. Providers are
// registered in attributeProviders keyed by qualified attribute
// name (e.g. `std.@secretkey`) and dispatched by
// dispatchAttributeProvider when the cursor sits inside the
// parameter's argument.
type AttributeCompletionProvider func(
	s *Server,
	docURI protocol.DocumentURI,
	cur CursorContext,
) []protocol.CompletionItem

// attributeProviders maps qualified attribute type names to their
// LSP completion providers. This is the LSP-side mirror of the
// linker's AttributeRegistry — same key, different value (UX
// instead of validation behaviour).
var attributeProviders = map[string]AttributeCompletionProvider{
	"std.@secretkey": (*Server).completeSecretKeysProvider,
}

// completeSecretKeysProvider is the AttributeCompletionProvider
// adapter around the existing completeSecretKeys helper. The shape
// difference exists because the historical helper takes a method
// receiver — wrapping it as a free function with explicit *Server
// keeps it usable from the providers map.
func (s *Server) completeSecretKeysProvider(docURI protocol.DocumentURI, cur CursorContext) []protocol.CompletionItem {
	return s.completeSecretKeys(docURI, cur)
}

// dispatchAttributeProvider walks the active parameter's attributes,
// looks up a registered provider for each, and returns the first
// non-empty result. Returns nil if no provider matches — callers
// should fall back to other completion strategies.
func (s *Server) dispatchAttributeProvider(docURI protocol.DocumentURI, cur CursorContext) []protocol.CompletionItem {
	f, ok := s.lookupFunc(docURI, cur.FuncName)
	if !ok {
		return nil
	}
	param, ok := activeParam(f, cur)
	if !ok {
		return nil
	}
	for _, a := range param.Attributes {
		provider, ok := attributeProviders[a.QualifiedName]
		if !ok {
			continue
		}
		if items := provider(s, docURI, cur); len(items) > 0 {
			return items
		}
	}
	return nil
}

// activeParam returns the parameter the cursor is currently editing
// the value of, identified by ActiveKwarg (named) or ActiveParam
// (positional index). Returns false if neither identifies a param.
func activeParam(f FuncInfo, cur CursorContext) (ParamInfo, bool) {
	if cur.ActiveKwarg != "" {
		for _, p := range f.Params {
			if p.Name == cur.ActiveKwarg {
				return p, true
			}
		}
		return ParamInfo{}, false
	}
	if cur.ActiveParam >= 0 && cur.ActiveParam < len(f.Params) {
		return f.Params[cur.ActiveParam], true
	}
	return ParamInfo{}, false
}

// Type-based kwarg value completions
// -----------------------------------------------------------------------------

// completionForReturnType builds a completion item for a catalog entry that
// returns the requested type. Decls (which take struct-literal bodies) get
// `name {` insertion; funcs get `name(` insertion.
func (s *Server) completionForReturnType(name string) protocol.CompletionItem {
	f, _ := s.catalog.Lookup(name)
	insert := name + "("
	if isDecl(f) {
		insert = name + " {"
	}
	return protocol.CompletionItem{
		Label:      name,
		Kind:       protocol.CompletionItemKindFunction,
		Detail:     f.Summary,
		InsertText: insert,
	}
}

// isDecl reports whether a FuncInfo represents a `decl` (struct-literal body)
// rather than a `func` (call expression). Decls return a non-builtin type but
// don't take a parenthesised arg list at the call site — they're invoked with
// brace bodies. The catalog stores both as FuncInfo, so we infer from naming
// convention used in the std stubs: decls are the things callers reach for
// when they need a value of a specific type.
//
// In the current std/, decls are distinguished by being the only non-step
// FuncInfo entries with a non-empty ReturnType (funcs may also have one, but
// none of the type-targeted constructors are funcs today). If a future func
// produces a typed value, this check needs to honour an explicit kind flag.
func isDecl(f FuncInfo) bool {
	return f.ReturnType != "" && !f.IsStep
}

// completeKwargValue offers type-appropriate values for a kwarg being typed.
// Looks up all catalog entries whose return type matches the param type and
// offers them as completions. This works uniformly for typed constructors
// (posix.source_local for posix.Source, posix.pkg_system for posix.PkgSource)
// and for value-producer funcs (std.secret, std.env for string).
func (s *Server) completeKwargValue(docURI protocol.DocumentURI, cur CursorContext) []protocol.CompletionItem {
	f, ok := s.lookupFunc(docURI, cur.FuncName)
	if !ok {
		return nil
	}

	for _, p := range f.Params {
		if p.Name != cur.ActiveKwarg {
			continue
		}
		// Strip optional `?` so `posix.Source?` matches `posix.Source`.
		typeName := strings.TrimSuffix(p.Type, "?")
		names := s.catalog.ByReturnType(typeName)
		if len(names) == 0 {
			return nil
		}
		items := make([]protocol.CompletionItem, 0, len(names))
		for _, n := range names {
			items = append(items, s.completionForReturnType(n))
		}
		return items
	}
	return nil
}

// completeEnumValues offers valid enum values for a kwarg being typed.
func (s *Server) completeEnumValues(docURI protocol.DocumentURI, cur CursorContext) []protocol.CompletionItem {
	f, ok := s.lookupFunc(docURI, cur.FuncName)
	if !ok {
		return nil
	}

	for _, p := range f.Params {
		if p.Name != cur.ActiveKwarg || len(p.EnumValues) == 0 {
			continue
		}

		var items []protocol.CompletionItem
		for _, v := range p.EnumValues {
			if cur.WordUnderCursor != "" && !strings.HasPrefix(v, cur.WordUnderCursor) {
				continue
			}
			items = append(items, protocol.CompletionItem{
				Label:      v,
				Kind:       protocol.CompletionItemKindEnumMember,
				InsertText: v,
			})
		}
		return items
	}
	return nil
}

// completeSecretKeys offers secret key names from the configured secrets file.
func (s *Server) completeSecretKeys(docURI protocol.DocumentURI, cur CursorContext) []protocol.CompletionItem {
	doc, ok := s.docs.Get(docURI)
	if !ok {
		return nil
	}

	filePath := uriToPath(docURI)
	secretsPath := findSecretsPath(filePath, doc.Content)
	if secretsPath == "" {
		return nil
	}

	data, err := os.ReadFile(secretsPath)
	if err != nil {
		return nil
	}

	var secrets map[string]any
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil
	}

	var items []protocol.CompletionItem
	for key := range secrets {
		if cur.WordUnderCursor != "" && !strings.HasPrefix(key, cur.WordUnderCursor) {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label:      key,
			Kind:       protocol.CompletionItemKindProperty,
			InsertText: key,
		})
	}
	return items
}

// findSecretsPath extracts the path argument from a secrets() call in the
// document and resolves it relative to the file's directory.
func findSecretsPath(filePath, content string) string {
	// Try AST-based extraction first.
	if p := findSecretsPathAST(filePath, content); p != "" {
		return p
	}
	// Fallback: scan raw text for secrets(... path="..." ...).
	return findSecretsPathText(filePath, content)
}

func findSecretsPathAST(filePath, content string) string {
	f, _ := Parse(filePath, []byte(content))
	if f == nil {
		return ""
	}

	var secretsPath string
	ast.Walk(f, func(n ast.Node) bool {
		if n == nil || secretsPath != "" {
			return false
		}
		// Look for a struct-lit invocation of "secrets" — in the new lang
		// secrets config is a decl invocation: secrets { path = "...", ... }
		sl, ok := n.(*ast.StructLit)
		if !ok {
			return true
		}
		if sl.Type == nil {
			return true
		}
		name := typeExprString(sl.Type)
		if name != "secrets" && name != "std.secrets" {
			return true
		}
		for _, fi := range sl.Fields {
			if fi.Name.Name == "path" {
				if str := stringLitValue(fi.Value); str != "" {
					secretsPath = str
				}
			}
		}
		return true
	}, nil)

	if secretsPath == "" {
		return ""
	}
	return resolveSecretsPath(filePath, secretsPath)
}

func stringLitValue(e ast.Expr) string {
	sl, ok := e.(*ast.StringLit)
	if !ok || len(sl.Parts) != 1 {
		return ""
	}
	if text, ok := sl.Parts[0].(*ast.StringText); ok {
		return text.Raw
	}
	return ""
}

func findSecretsPathText(filePath, content string) string {
	// Look for: path="something.json" near a secrets( call.
	idx := strings.Index(content, "secrets")
	if idx < 0 {
		return ""
	}
	rest := content[idx:]
	if len(rest) > 200 {
		rest = rest[:200]
	}
	const marker = `path = "`
	pIdx := strings.Index(rest, marker)
	if pIdx < 0 {
		// Try without spaces.
		const marker2 = `path="`
		pIdx = strings.Index(rest, marker2)
		if pIdx < 0 {
			return ""
		}
		start := pIdx + len(marker2)
		end := strings.IndexByte(rest[start:], '"')
		if end < 0 {
			return ""
		}
		return resolveSecretsPath(filePath, rest[start:start+end])
	}
	start := pIdx + len(marker)
	end := strings.IndexByte(rest[start:], '"')
	if end < 0 {
		return ""
	}
	return resolveSecretsPath(filePath, rest[start:start+end])
}

func resolveSecretsPath(filePath, secretsPath string) string {
	if filepath.IsAbs(secretsPath) {
		return secretsPath
	}
	return filepath.Join(filepath.Dir(filePath), secretsPath)
}

func isDotPrefix(word string) bool {
	return strings.Contains(word, ".")
}
