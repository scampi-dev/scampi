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

func (s *Server) DocumentSymbol(
	_ context.Context,
	params *protocol.DocumentSymbolParams,
) ([]any, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	filePath := uriToPath(params.TextDocument.URI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil, nil
	}

	data := []byte(doc.Content)
	var symbols []any
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			symbols = append(symbols, protocol.DocumentSymbol{
				Name:           d.Name.Name,
				Kind:           protocol.SymbolKindFunction,
				Range:          tokenSpanToRange(data, d.SrcSpan),
				SelectionRange: tokenSpanToRange(data, d.Name.SrcSpan),
			})
		case *ast.DeclDecl:
			name := declName(d)
			symbols = append(symbols, protocol.DocumentSymbol{
				Name:           name,
				Kind:           protocol.SymbolKindFunction,
				Range:          tokenSpanToRange(data, d.SrcSpan),
				SelectionRange: tokenSpanToRange(data, d.Name.SrcSpan),
			})
		case *ast.LetDecl:
			symbols = append(symbols, protocol.DocumentSymbol{
				Name:           d.Name.Name,
				Kind:           protocol.SymbolKindVariable,
				Range:          tokenSpanToRange(data, d.SrcSpan),
				SelectionRange: tokenSpanToRange(data, d.Name.SrcSpan),
			})
		case *ast.TypeDecl:
			symbols = append(symbols, protocol.DocumentSymbol{
				Name:           d.Name.Name,
				Kind:           protocol.SymbolKindStruct,
				Range:          tokenSpanToRange(data, d.SrcSpan),
				SelectionRange: tokenSpanToRange(data, d.Name.SrcSpan),
			})
		case *ast.EnumDecl:
			symbols = append(symbols, protocol.DocumentSymbol{
				Name:           d.Name.Name,
				Kind:           protocol.SymbolKindEnum,
				Range:          tokenSpanToRange(data, d.SrcSpan),
				SelectionRange: tokenSpanToRange(data, d.Name.SrcSpan),
			})
		}
	}

	return symbols, nil
}

func (s *Server) Symbols(
	_ context.Context,
	params *protocol.WorkspaceSymbolParams,
) ([]protocol.SymbolInformation, error) {
	if s.rootDir == "" {
		return nil, nil
	}

	query := strings.ToLower(params.Query)
	var symbols []protocol.SymbolInformation

	// Scan workspace root.
	symbols = appendSymbolsFromDir(symbols, s.rootDir, query)

	// Scan local dependency directories from scampi.mod.
	if s.module != nil {
		for _, dep := range s.module.Require {
			if !dep.IsLocal() {
				continue
			}
			depDir := dep.Version
			if !filepath.IsAbs(depDir) {
				depDir = filepath.Join(filepath.Dir(s.module.Filename), depDir)
			}
			symbols = appendSymbolsFromDir(symbols, depDir, query)
		}
	}

	return symbols, nil
}

func appendSymbolsFromDir(symbols []protocol.SymbolInformation, dir, query string) []protocol.SymbolInformation {
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".scampi" {
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

		for _, d := range f.Decls {
			var name string
			var kind protocol.SymbolKind
			var span token.Span

			switch d := d.(type) {
			case *ast.FuncDecl:
				name = d.Name.Name
				kind = protocol.SymbolKindFunction
				span = d.Name.SrcSpan
			case *ast.LetDecl:
				name = d.Name.Name
				kind = protocol.SymbolKindVariable
				span = d.Name.SrcSpan
			case *ast.DeclDecl:
				if len(d.Name.Parts) > 0 {
					name = d.Name.Parts[0].Name
				}
				kind = protocol.SymbolKindFunction
				span = d.Name.SrcSpan
			case *ast.TypeDecl:
				name = d.Name.Name
				kind = protocol.SymbolKindStruct
				span = d.Name.SrcSpan
			case *ast.EnumDecl:
				name = d.Name.Name
				kind = protocol.SymbolKindEnum
				span = d.Name.SrcSpan
			}

			if name == "" {
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(name), query) {
				continue
			}

			symbols = append(symbols, protocol.SymbolInformation{
				Name: name,
				Kind: kind,
				Location: protocol.Location{
					URI:   uri.File(path),
					Range: tokenSpanToRange(data, span),
				},
			})
		}
		return nil
	})
	return symbols
}
