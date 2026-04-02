// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"os"
	"path/filepath"

	"go.lsp.dev/protocol"
	"go.starlark.net/syntax"
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

	// Skip builtins.
	if _, ok := s.catalog.Lookup(word); ok {
		return nil, nil
	}

	var locs []protocol.Location

	// All references in the current file.
	locs = append(locs, findIdents(f, filePath, word)...)

	// If the symbol comes from a load(), search the loaded file.
	if resolved := loadSourceForName(f, filePath, word); resolved != "" {
		locs = append(locs, refsInFile(resolved, word)...)
	}

	// If the symbol is defined here, search files that load this file.
	if findDefinition(f, word) != nil {
		locs = append(locs, s.refsFromLoaders(filePath, word)...)
	}

	return dedup(locs, locationKey), nil
}

// findIdents walks the AST and returns locations of every Ident matching name.
func findIdents(f *syntax.File, filePath, name string) []protocol.Location {
	var locs []protocol.Location
	syntax.Walk(f, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		if id, ok := n.(*syntax.Ident); ok && id.Name == name {
			locs = append(locs, posToLocation(filePath, id.NamePos))
		}
		return true
	})
	return locs
}

// loadSourceForName checks if name is imported via load() and returns the
// resolved file path and original name in that file.
func loadSourceForName(f *syntax.File, filePath, name string) string {
	for _, stmt := range f.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}
		for i, to := range load.To {
			if to.Name != name {
				continue
			}
			targetName := to.Name
			if i < len(load.From) {
				targetName = load.From[i].Name
			}
			_ = targetName // same name lookup in external file
			return resolveLoadPath(filePath, load.ModuleName())
		}
	}
	return ""
}

// refsFromLoaders finds .scampi files in the same directory (and parent
// directories up to the project root) that load the given file, and
// searches them for references to name.
func (s *Server) refsFromLoaders(filePath, name string) []protocol.Location {
	var locs []protocol.Location
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)

	candidates := scampiFilesIn(dir)
	for _, c := range candidates {
		if c == filePath {
			continue
		}
		if loadsFile(c, base) {
			locs = append(locs, refsInFile(c, name)...)
		}
	}
	return locs
}

func refsInFile(path, name string) []protocol.Location {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	f, _ := Parse(path, data)
	if f == nil {
		return nil
	}
	return findIdents(f, path, name)
}

// loadsFile checks whether the Starlark file at candidate contains a
// load() statement referencing target (a basename in the same directory).
func loadsFile(candidate, target string) bool {
	data, err := os.ReadFile(candidate)
	if err != nil {
		return false
	}
	f, _ := Parse(candidate, data)
	if f == nil {
		return false
	}
	targetAbs := filepath.Join(filepath.Dir(candidate), target)
	for _, stmt := range f.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}
		if resolveLoadPath(candidate, load.ModuleName()) == targetAbs {
			return true
		}
	}
	return false
}

// scampiFilesIn returns all .scampi files in the given directory.
func scampiFilesIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".scampi" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}
