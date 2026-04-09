// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"

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

	word := wordAtOffset(doc.Content, offsetFromPosition(
		doc.Content,
		params.Position.Line,
		params.Position.Character,
	))
	if word == "" {
		return nil, nil
	}

	// Search current file for definition first.
	data := []byte(doc.Content)
	if span := findDefinition(f, word); span != nil {
		return []protocol.Location{spanToLocation(filePath, data, *span)}, nil
	}

	// Stdlib — resolve to extracted stub file.
	if loc, ok := s.stubDefs.Lookup(word); ok {
		return []protocol.Location{loc}, nil
	}

	return nil, nil
}

// findDefinition searches top-level declarations for a name.
func findDefinition(f *ast.File, name string) *token.Span {
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
