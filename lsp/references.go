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

	switch {
	case strings.HasPrefix(word, "@"):
		// Attribute reference (e.g. `@secretkey`, `@std.path`).
		// Search for matching uses on Field.Attributes everywhere
		// and include the AttrTypeDecl def site.
		locs = append(locs, findAttrRefs(f, filePath, data, word)...)
		locs = append(locs, findAttrDef(f, filePath, data, word)...)
		locs = append(locs, s.attrRefsInAllDirs(word, filePath)...)
		if loc, ok := s.stubDefs.Lookup(word); ok {
			locs = append(locs, loc)
		}
	case strings.Contains(word, "."):
		// Dotted name (posix.pkg, std.Step, etc.) — search for
		// dotted references in AST nodes.
		locs = append(locs, findDottedRefs(f, filePath, data, word)...)
		locs = append(locs, s.refsInAllDirs(word, filePath)...)
		if loc, ok := s.stubDefs.Lookup(word); ok {
			locs = append(locs, loc)
		}
	default:
		// Bare name — search current file for ident matches.
		locs = append(locs, findIdents(f, filePath, data, word)...)
		// Multi-file module: also search sibling files for bare
		// ident matches (same-package visibility).
		modName := fileModuleName(f)
		if modName != "" && modName != "main" {
			locs = append(locs, s.refsInSiblings(filePath, modName, word)...)
		}
		// Also search for the qualified form (e.g. "pkg" →
		// "posix.pkg") across all workspace dirs.
		if modName != "" {
			qualified := modName + "." + word
			locs = append(locs, s.refsInAllDirs(qualified, filePath)...)
		}
	}

	return dedup(locs, locationKey), nil
}

// findAttrRefs returns the location of every Attribute reference in
// the file whose name matches the given `@`-prefixed word. Both
// single-segment (`@secretkey`) and qualified (`@std.path`) forms
// are supported; for single-segment lookup any matching last segment
// counts as a hit, mirroring how the type checker resolves names
// through scope.
func findAttrRefs(f *ast.File, filePath string, src []byte, word string) []protocol.Location {
	bare, qualified := splitAttrWord(word)
	var locs []protocol.Location
	ast.Walk(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		a, ok := n.(*ast.Attribute)
		if !ok {
			return true
		}
		if !attrNameMatches(a.Name, bare, qualified) {
			return true
		}
		locs = append(locs, protocol.Location{
			URI:   uri.File(filePath),
			Range: tokenSpanToRange(src, a.Name.SrcSpan),
		})
		return true
	}, nil)
	return locs
}

// findAttrDef returns the location of the AttrTypeDecl in this file
// that defines the given attribute reference. Returns no locations if
// the def lives in a stub or another file (the caller picks that up
// via stubDefs.Lookup or attrRefsInAllDirs).
func findAttrDef(f *ast.File, filePath string, src []byte, word string) []protocol.Location {
	bare, _ := splitAttrWord(word)
	var locs []protocol.Location
	for _, d := range f.Decls {
		atd, ok := d.(*ast.AttrTypeDecl)
		if !ok {
			continue
		}
		if atd.Name.Name == bare {
			locs = append(locs, protocol.Location{
				URI:   uri.File(filePath),
				Range: tokenSpanToRange(src, atd.Name.SrcSpan),
			})
		}
	}
	return locs
}

// attrRefsInAllDirs walks the workspace root and any user module
// directories to find Attribute references matching the given word.
func (s *Server) attrRefsInAllDirs(word, excludePath string) []protocol.Location {
	var locs []protocol.Location
	seen := map[string]bool{}

	if s.rootDir != "" {
		seen[s.rootDir] = true
		locs = append(locs, attrRefsInDir(s.rootDir, word, excludePath)...)
	}
	if s.module != nil {
		for _, dep := range s.module.Require {
			dir := depDir(s.module, &dep)
			abs, _ := filepath.Abs(dir)
			if abs != "" {
				dir = abs
			}
			if seen[dir] {
				continue
			}
			seen[dir] = true
			locs = append(locs, attrRefsInDir(dir, word, excludePath)...)
		}
	}
	return locs
}

func attrRefsInDir(dir, word, excludePath string) []protocol.Location {
	var locs []protocol.Location
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
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
		locs = append(locs, findAttrRefs(f, path, data, word)...)
		return nil
	})
	return locs
}

// splitAttrWord splits an attribute reference word into its bare and
// qualified forms. Examples:
//
//	"@secretkey"     → "secretkey", ""           (single segment)
//	"@std.secretkey" → "secretkey", "std.secretkey"  (qualified)
func splitAttrWord(word string) (bare, qualified string) {
	stripped := strings.TrimPrefix(word, "@")
	if i := strings.LastIndexByte(stripped, '.'); i >= 0 {
		return stripped[i+1:], stripped
	}
	return stripped, ""
}

// attrNameMatches reports whether an attribute reference's DottedName
// matches the requested bare/qualified pair. Single-segment refs
// match any attribute whose final segment is bare; qualified refs
// require an exact dotted match.
func attrNameMatches(name *ast.DottedName, bare, qualified string) bool {
	if qualified != "" {
		return dottedString(name) == qualified
	}
	if len(name.Parts) == 0 {
		return false
	}
	return name.Parts[len(name.Parts)-1].Name == bare
}

// refsInAllDirs searches the workspace root and all user module
// directories for references to the given qualified name.
func (s *Server) refsInAllDirs(qualifiedName, excludePath string) []protocol.Location {
	var locs []protocol.Location
	seen := map[string]bool{} // avoid scanning a dir twice

	// Workspace root.
	if s.rootDir != "" {
		seen[s.rootDir] = true
		locs = append(locs, refsInDir(s.rootDir, qualifiedName, excludePath)...)
	}

	// User module directories.
	if s.module != nil {
		for _, dep := range s.module.Require {
			dir := depDir(s.module, &dep)
			abs, _ := filepath.Abs(dir)
			if abs != "" {
				dir = abs
			}
			if seen[dir] {
				continue
			}
			seen[dir] = true
			locs = append(locs, refsInDir(dir, qualifiedName, excludePath)...)
		}
	}

	return locs
}

func refsInDir(dir, qualifiedName, excludePath string) []protocol.Location {
	var locs []protocol.Location
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
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

// refsInSiblings searches sibling .scampi files in the same module
// directory for bare ident references. Used by find-references in
// multi-file modules (e.g. finding uses of get_nginx_certificates
// defined in api.scampi from within _index.scampi).
func (s *Server) refsInSiblings(
	filePath string,
	modName string,
	word string,
) []protocol.Location {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var locs []protocol.Location
	for _, e := range entries {
		if e.IsDir() || e.Name() == base {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".scampi") {
			continue
		}
		sibPath := filepath.Join(dir, e.Name())
		sibData, err := os.ReadFile(sibPath)
		if err != nil {
			continue
		}
		sibFile, _ := Parse(sibPath, sibData)
		if sibFile == nil || fileModuleName(sibFile) != modName {
			continue
		}
		locs = append(locs, findIdents(sibFile, sibPath, sibData, word)...)
	}
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
