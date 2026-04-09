// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"go.lsp.dev/protocol"

	"scampi.dev/scampi/lang/ast"
)

// lookupFunc checks the stdlib catalog first, then falls back to resolving
// user-defined functions from the current file.
func (s *Server) lookupFunc(docURI protocol.DocumentURI, name string) (FuncInfo, bool) {
	if f, ok := s.catalog.Lookup(name); ok {
		return f, true
	}
	return s.resolveUserFunc(docURI, name)
}

// resolveUserFunc attempts to find a user-defined function by name in the
// current file. Returns a FuncInfo with params extracted from the
// FuncDecl, or false if not found.
func (s *Server) resolveUserFunc(docURI protocol.DocumentURI, name string) (FuncInfo, bool) {
	doc, ok := s.docs.Get(docURI)
	if !ok {
		return FuncInfo{}, false
	}

	filePath := uriToPath(docURI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return FuncInfo{}, false
	}

	if bf, ok := funcDeclToLSP(f, name); ok {
		return bf, true
	}

	return FuncInfo{}, false
}

// funcDeclToLSP finds a FuncDecl by name and converts it to FuncInfo.
func funcDeclToLSP(f *ast.File, name string) (FuncInfo, bool) {
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != name {
			continue
		}

		params := fieldsToParams(fd.Params, "")
		return FuncInfo{
			Name:   name,
			Params: params,
		}, true
	}
	return FuncInfo{}, false
}
