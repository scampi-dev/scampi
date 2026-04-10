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
	case cur.InCall && cur.FuncName == "std.secret" && cur.InString:
		items = s.completeSecretKeys(params.TextDocument.URI, cur)
	case cur.InCall && cur.ActiveKwarg != "" && cur.InString:
		items = s.completeEnumValues(params.TextDocument.URI, cur)
	case cur.InCall && cur.ActiveKwarg != "":
		items = s.completeKwargValue(params.TextDocument.URI, cur)
	case cur.InList:
		items = s.completeTopLevel(cur.WordUnderCursor)
	case cur.InCall:
		items = s.completeKwargs(params.TextDocument.URI, cur)
	case isDotPrefix(cur.WordUnderCursor):
		items = s.completeModule(cur.WordUnderCursor)
	default:
		items = s.completeTopLevel(cur.WordUnderCursor)
	}

	s.log.Printf("completion: returning %d items", len(items))
	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}

// completeTopLevel offers all top-level names (non-dotted and module names).
func (s *Server) completeTopLevel(prefix string) []protocol.CompletionItem {
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

	return items
}

// completeModule offers members of a dotted module prefix (e.g. "posix." → "copy", "dir", ...).
func (s *Server) completeModule(word string) []protocol.CompletionItem {
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
	return items
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
