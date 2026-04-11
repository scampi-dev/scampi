// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
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
	if word == "" {
		return nil, nil
	}

	data := []byte(doc.Content)

	// Struct-literal field reference: cursor sits on a field name
	// inside a `Type { field = ... }` invocation. Resolve the type
	// to a stub decl/func/type and jump to the matching parameter
	// declaration. Detected by InCall (analyzeBraceContext sets it
	// for struct-lit braces) plus a known func name.
	if cur.InCall && cur.FuncName != "" && !strings.ContainsAny(word, ".") {
		if loc, ok := s.stubDefs.LookupParam(cur.FuncName, word); ok {
			return []protocol.Location{loc}, nil
		}
	}

	// Search current file for definition first.
	if span := findDefinition(f, word); span != nil {
		return []protocol.Location{spanToLocation(filePath, data, *span)}, nil
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

// findDefinition searches top-level declarations for a name.
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
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == name {
				s := d.Name.SrcSpan
				return &s
			}
		case *ast.DeclDecl:
			if len(d.Name.Parts) > 0 && d.Name.Parts[0].Name == name {
				s := d.Name.SrcSpan
				return &s
			}
		case *ast.LetDecl:
			if d.Name.Name == name {
				s := d.Name.SrcSpan
				return &s
			}
		case *ast.TypeDecl:
			if d.Name.Name == name {
				s := d.Name.SrcSpan
				return &s
			}
		case *ast.EnumDecl:
			if d.Name.Name == name {
				s := d.Name.SrcSpan
				return &s
			}
		}
	}
	return nil
}

func spanToLocation(path string, src []byte, s token.Span) protocol.Location {
	return protocol.Location{
		URI:   uri.File(path),
		Range: tokenSpanToRange(src, s),
	}
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
