// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"os"
	"path/filepath"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.starlark.net/syntax"
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
	line := params.Position.Line + 1 // LSP 0-based → Starlark 1-based
	col := params.Position.Character + 1

	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil, nil
	}

	s.log.Printf(
		"definition: %s L%d:%d",
		filePath,
		line,
		col,
	)

	// Check if cursor is inside a load() statement.
	if loc := s.definitionFromLoad(f, filePath, line, col); loc != nil {
		return []protocol.Location{*loc}, nil
	}

	// Find the identifier under the cursor.
	word := wordAtOffset(doc.Content, offsetFromPosition(
		doc.Content,
		params.Position.Line,
		params.Position.Character,
	))
	if word == "" {
		return nil, nil
	}

	// Skip builtins — they have no source definition.
	if _, ok := s.catalog.Lookup(word); ok {
		return nil, nil
	}

	// Search current file for definition.
	if pos := findDefinition(f, word); pos != nil {
		return []protocol.Location{posToLocation(filePath, *pos)}, nil
	}

	// Search loaded files.
	if loc := s.definitionFromLoads(f, filePath, word); loc != nil {
		return []protocol.Location{*loc}, nil
	}

	return nil, nil
}

// Load statements
// -----------------------------------------------------------------------------

func (s *Server) definitionFromLoad(
	f *syntax.File,
	filePath string,
	line, col uint32,
) *protocol.Location {
	for _, stmt := range f.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}
		start, end := load.Span()
		if line < uint32(start.Line) || line > uint32(end.Line) {
			continue
		}

		// Cursor on the module path string?
		modStart, modEnd := load.Module.Span()
		if posInSpan(line, col, modStart, modEnd) {
			resolved := resolveLoadPath(filePath, load.ModuleName())
			return fileLocation(resolved)
		}

		// Cursor on one of the imported symbol names?
		for i, to := range load.To {
			toStart, toEnd := to.Span()
			if posInSpan(line, col, toStart, toEnd) {
				resolved := resolveLoadPath(filePath, load.ModuleName())
				targetName := to.Name
				if i < len(load.From) {
					targetName = load.From[i].Name
				}
				return definitionInExternalFile(resolved, targetName)
			}
		}
	}
	return nil
}

func (s *Server) definitionFromLoads(
	f *syntax.File,
	filePath string,
	name string,
) *protocol.Location {
	for _, stmt := range f.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}
		for i, to := range load.To {
			if to.Name != name {
				continue
			}
			resolved := resolveLoadPath(filePath, load.ModuleName())
			targetName := to.Name
			if i < len(load.From) {
				targetName = load.From[i].Name
			}
			return definitionInExternalFile(resolved, targetName)
		}
	}
	return nil
}

// AST search
// -----------------------------------------------------------------------------

func findDefinition(f *syntax.File, name string) *syntax.Position {
	for _, stmt := range f.Stmts {
		switch s := stmt.(type) {
		case *syntax.DefStmt:
			if s.Name.Name == name {
				pos := s.Name.NamePos
				return &pos
			}
		case *syntax.AssignStmt:
			if ident, ok := s.LHS.(*syntax.Ident); ok && ident.Name == name {
				pos := ident.NamePos
				return &pos
			}
		}
	}
	return nil
}

func definitionInExternalFile(path, name string) *protocol.Location {
	if path == "" {
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
	pos := findDefinition(f, name)
	if pos == nil {
		return nil
	}
	loc := posToLocation(path, *pos)
	return &loc
}

// Path resolution
// -----------------------------------------------------------------------------

func resolveLoadPath(currentFile, loadPath string) string {
	if filepath.IsAbs(loadPath) {
		return loadPath
	}
	resolved := filepath.Join(filepath.Dir(currentFile), loadPath)
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return resolved
	}
	return abs
}

// Position helpers
// -----------------------------------------------------------------------------

func posInSpan(line, col uint32, start, end syntax.Position) bool {
	if line < uint32(start.Line) || line > uint32(end.Line) {
		return false
	}
	if line == uint32(start.Line) && col < uint32(start.Col) {
		return false
	}
	if line == uint32(end.Line) && col > uint32(end.Col) {
		return false
	}
	return true
}

func posToLocation(path string, pos syntax.Position) protocol.Location {
	return protocol.Location{
		URI:   uri.File(path),
		Range: posToLSPRange(pos),
	}
}

func posToLSPRange(pos syntax.Position) protocol.Range {
	line := uint32(0)
	if pos.Line > 0 {
		line = uint32(pos.Line - 1)
	}
	col := uint32(0)
	if pos.Col > 0 {
		col = uint32(pos.Col - 1)
	}
	return protocol.Range{
		Start: protocol.Position{Line: line, Character: col},
		End:   protocol.Position{Line: line, Character: col},
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

func fileLocation(path string) *protocol.Location {
	if path == "" {
		return nil
	}
	return &protocol.Location{
		URI:   uri.File(path),
		Range: protocol.Range{},
	}
}
