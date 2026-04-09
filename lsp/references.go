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
)

func (s *Server) References(
	_ context.Context,
	params *protocol.ReferenceParams,
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

	word := wordAtOffset(doc.Content, offsetFromPosition(
		doc.Content,
		params.Position.Line,
		params.Position.Character,
	))
	if word == "" {
		return nil, nil
	}

	s.log.Printf("references: %s %q", filePath, word)

	data := []byte(doc.Content)
	var locs []protocol.Location

	if strings.Contains(word, ".") {
		// Dotted name (posix.pkg, std.Step, etc.) — search for
		// dotted references in AST nodes.
		locs = append(locs, findDottedRefs(f, filePath, data, word)...)
		if s.rootDir != "" {
			locs = append(locs, s.refsInWorkspace(word, filePath)...)
		}
		if loc, ok := s.stubDefs.Lookup(word); ok {
			locs = append(locs, loc)
		}
	} else {
		// Bare name — search current file for ident matches.
		locs = append(locs, findIdents(f, filePath, data, word)...)
		// If we're in a stub file, also search workspace for the
		// qualified form (e.g. "pkg" → "posix.pkg").
		if modName := fileModuleName(f); modName != "" {
			qualified := modName + "." + word
			if s.rootDir != "" {
				locs = append(locs, s.refsInWorkspace(qualified, filePath)...)
			}
		}
	}

	return dedup(locs, locationKey), nil
}

// refsInWorkspace walks all .scampi files under the workspace root and
// finds references to the given qualified name (e.g. "posix.pkg").
// excludePath is skipped (already searched by the caller).
func (s *Server) refsInWorkspace(qualifiedName, excludePath string) []protocol.Location {
	var locs []protocol.Location
	_ = filepath.WalkDir(s.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".scampi" || path == excludePath {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		f, _ := Parse(path, data)
		if f == nil {
			return nil
		}
		locs = append(locs, findDottedRefs(f, path, data, qualifiedName)...)
		return nil
	})
	return locs
}

// findIdents walks the AST and returns locations of every Ident matching name.
func findIdents(f *ast.File, filePath string, src []byte, name string) []protocol.Location {
	var locs []protocol.Location
	ast.Walk(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			locs = append(locs, protocol.Location{
				URI:   uri.File(filePath),
				Range: tokenSpanToRange(src, id.SrcSpan),
			})
		}
		return true
	}, nil)
	return locs
}

// findDottedRefs finds references to a qualified name like "posix.pkg"
// in the AST. Matches StructLit type references (posix.pkg { ... }),
// NamedType references (types in annotations), and DottedName nodes.
func findDottedRefs(f *ast.File, filePath string, src []byte, qualifiedName string) []protocol.Location {
	var locs []protocol.Location
	ast.Walk(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		switch n := n.(type) {
		case *ast.StructLit:
			if n.Type != nil && typeExprString(n.Type) == qualifiedName {
				locs = append(locs, protocol.Location{
					URI:   uri.File(filePath),
					Range: tokenSpanToRange(src, n.Type.Span()),
				})
			}
		case *ast.NamedType:
			if dottedString(n.Name) == qualifiedName {
				locs = append(locs, protocol.Location{
					URI:   uri.File(filePath),
					Range: tokenSpanToRange(src, n.SrcSpan),
				})
			}
		case *ast.DottedName:
			if dottedString(n) == qualifiedName {
				locs = append(locs, protocol.Location{
					URI:   uri.File(filePath),
					Range: tokenSpanToRange(src, n.SrcSpan),
				})
			}
		case *ast.SelectorExpr:
			// posix.pkg in expression context is parsed as SelectorExpr
			if selectorString(n) == qualifiedName {
				locs = append(locs, protocol.Location{
					URI:   uri.File(filePath),
					Range: tokenSpanToRange(src, n.SrcSpan),
				})
			}
		}
		return true
	}, nil)
	return locs
}

// fileModuleName returns the module name from the file's module
// declaration, or "" if there is none.
func fileModuleName(f *ast.File) string {
	if f.Module != nil {
		return f.Module.Name.Name
	}
	return ""
}

func dottedString(dn *ast.DottedName) string {
	var parts []string
	for _, p := range dn.Parts {
		parts = append(parts, p.Name)
	}
	return strings.Join(parts, ".")
}

func selectorString(sel *ast.SelectorExpr) string {
	if id, ok := sel.X.(*ast.Ident); ok {
		return id.Name + "." + sel.Sel.Name
	}
	return ""
}
