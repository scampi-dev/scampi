// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/token"
)

func (s *Server) Definition(
	_ context.Context,
	params *protocol.DefinitionParams,
) ([]protocol.Location, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	filePath := uriToPath(params.TextDocument.URI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil, nil
	}

	s.log.Printf(
		"definition: %s L%d:%d",
		filePath,
		params.Position.Line+1,
		params.Position.Character+1,
	)

	cur := AnalyzeCursor(doc.Content, params.Position.Line, params.Position.Character)
	word := cur.WordUnderCursor

	data := []byte(doc.Content)
	offset := byteOffsetAtPosition(doc.Content, params.Position.Line, params.Position.Character)

	// Import path: cursor inside `import "std/posix"` → jump to module file.
	for _, imp := range f.Imports {
		if offset >= int(imp.SrcSpan.Start) && offset <= int(imp.SrcSpan.End) {
			if loc, ok := s.stubDefs.LookupModule(imp.Path); ok {
				return []protocol.Location{loc}, nil
			}
		}
	}

	if word == "" {
		return nil, nil
	}

	// Struct-literal field reference: cursor sits on a field name
	// inside a `Type { field = ... }` invocation. Resolve the type
	// to a stub decl/func/type and jump to the matching parameter
	// declaration. Detected by InCall (analyzeBraceContext sets it
	// for struct-lit braces) plus a known func name.
	if cur.InCall && cur.FuncName != "" && !strings.ContainsAny(word, ".") {
		if loc, ok := s.stubDefs.LookupParam(cur.FuncName, word); ok {
			return []protocol.Location{loc}, nil
		}
		// User-defined type: search the current file's type/decl
		// declarations for a matching field name.
		if loc, ok := findFieldDefinition(f, filePath, data, cur.FuncName, word); ok {
			return []protocol.Location{loc}, nil
		}
	}

	// Search current file for definition first.
	if span := findDefinition(f, word); span != nil {
		return []protocol.Location{spanToLocation(filePath, data, *span)}, nil
	}

	// Multi-file module: search sibling .scampi files in the same
	// directory that share the same module declaration. This
	// handles goto-def from _index.scampi to api.scampi within
	// a module package.
	if f.Module != nil && f.Module.Name.Name != "main" {
		if loc, ok := s.findInSiblings(filePath, f.Module.Name.Name, word); ok {
			return []protocol.Location{loc}, nil
		}
	}

	// Stdlib — resolve to extracted stub file.
	if loc, ok := s.stubDefs.Lookup(word); ok {
		return []protocol.Location{loc}, nil
	}

	// Dotted word like `x.yo` from a UFCS or selector context — try
	// the trailing segment in the same order so the function-name
	// part of `x.yo()` resolves to local `yo`.
	if i := strings.LastIndexByte(word, '.'); i >= 0 && i < len(word)-1 {
		tail := word[i+1:]
		if span := findDefinition(f, tail); span != nil {
			return []protocol.Location{spanToLocation(filePath, data, *span)}, nil
		}
		if loc, ok := s.stubDefs.Lookup(tail); ok {
			return []protocol.Location{loc}, nil
		}
	}

	return nil, nil
}

// findFieldDefinition searches type/decl declarations for a field
// matching fieldName within the type named typeName. Returns the
// field's name span if found.
func findFieldDefinition(
	f *ast.File,
	filePath string,
	data []byte,
	typeName string,
	fieldName string,
) (protocol.Location, bool) {
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.TypeDecl:
			if d.Name.Name != typeName {
				continue
			}
			for _, field := range d.Fields {
				if field.Name != nil && field.Name.Name == fieldName {
					return spanToLocation(filePath, data, field.Name.SrcSpan), true
				}
			}
		case *ast.DeclDecl:
			if len(d.Name.Parts) == 0 || d.Name.Parts[0].Name != typeName {
				continue
			}
			for _, field := range d.Params {
				if field.Name != nil && field.Name.Name == fieldName {
					return spanToLocation(filePath, data, field.Name.SrcSpan), true
				}
			}
		case *ast.FuncDecl:
			if d.Name.Name != typeName {
				continue
			}
			for _, field := range d.Params {
				if field.Name != nil && field.Name.Name == fieldName {
					return spanToLocation(filePath, data, field.Name.SrcSpan), true
				}
			}
		}
	}
	return protocol.Location{}, false
}

// findDefinition searches the entire AST for a declaration matching name,
// including nested let bindings inside block bodies.
// Attribute references (with leading `@`) match AttrTypeDecls.
func findDefinition(f *ast.File, name string) *token.Span {
	if len(name) > 1 && name[0] == '@' {
		bare := name[1:]
		for _, d := range f.Decls {
			if atd, ok := d.(*ast.AttrTypeDecl); ok && atd.Name.Name == bare {
				s := atd.Name.SrcSpan
				return &s
			}
		}
		return nil
	}

	var result *token.Span
	ast.Walk(f, func(n ast.Node) bool {
		if result != nil {
			return false
		}
		switch d := n.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == name {
				s := d.Name.SrcSpan
				result = &s
			}
		case *ast.DeclDecl:
			if len(d.Name.Parts) > 0 && d.Name.Parts[0].Name == name {
				s := d.Name.SrcSpan
				result = &s
			}
		case *ast.LetDecl:
			if d.Name.Name == name {
				s := d.Name.SrcSpan
				result = &s
			}
		case *ast.TypeDecl:
			if d.Name.Name == name {
				s := d.Name.SrcSpan
				result = &s
			}
		case *ast.EnumDecl:
			if d.Name.Name == name {
				s := d.Name.SrcSpan
				result = &s
			}
		}
		return true
	}, nil)
	return result
}

// findInSiblings searches sibling .scampi files in the same directory
// for a declaration matching word. Used by goto-def in multi-file
// modules (e.g. _index.scampi → api.scampi).
func (s *Server) findInSiblings(
	filePath string,
	modName string,
	word string,
) (protocol.Location, bool) {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return protocol.Location{}, false
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == base {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".scampi") ||
			strings.HasSuffix(e.Name(), "_test.scampi") {
			continue
		}
		sibPath := filepath.Join(dir, e.Name())
		sibData, err := os.ReadFile(sibPath)
		if err != nil {
			continue
		}
		sibFile, _ := Parse(sibPath, sibData)
		if sibFile == nil || sibFile.Module == nil || sibFile.Module.Name.Name != modName {
			continue
		}
		if span := findDefinition(sibFile, word); span != nil {
			return spanToLocation(sibPath, sibData, *span), true
		}
	}
	return protocol.Location{}, false
}

func spanToLocation(path string, src []byte, s token.Span) protocol.Location {
	return protocol.Location{
		URI:   uri.File(path),
		Range: tokenSpanToRange(src, s),
	}
}

// declNameAndSpan extracts the name and name span from a declaration node.
func declNameAndSpan(d ast.Decl) (string, token.Span) {
	switch d := d.(type) {
	case *ast.FuncDecl:
		return d.Name.Name, d.Name.SrcSpan
	case *ast.DeclDecl:
		if len(d.Name.Parts) > 0 {
			return d.Name.Parts[0].Name, d.Name.SrcSpan
		}
	case *ast.LetDecl:
		return d.Name.Name, d.Name.SrcSpan
	case *ast.TypeDecl:
		return d.Name.Name, d.Name.SrcSpan
	case *ast.EnumDecl:
		return d.Name.Name, d.Name.SrcSpan
	}
	return "", token.Span{}
}

// dedup returns a slice with duplicates removed, using keyFn to determine
// identity. Order is preserved; first occurrence wins.
func dedup[S ~[]E, E any, K comparable](s S, keyFn func(E) K) S {
	seen := make(map[K]struct{}, len(s))
	out := s[:0]
	for _, v := range s {
		k := keyFn(v)
		if _, dup := seen[k]; !dup {
			seen[k] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

type locKey struct {
	uri        protocol.DocumentURI
	line, char uint32
}

func locationKey(loc protocol.Location) locKey {
	return locKey{loc.URI, loc.Range.Start.Line, loc.Range.Start.Character}
}

func byteOffsetAtPosition(content string, line, char uint32) int {
	offset := 0
	for l := uint32(0); l < line && offset < len(content); offset++ {
		if content[offset] == '\n' {
			l++
		}
	}
	return offset + int(char)
}
