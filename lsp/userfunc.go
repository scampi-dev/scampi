// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"os"

	"go.lsp.dev/protocol"
	"go.starlark.net/syntax"
)

// lookupFunc checks the builtin catalog first, then falls back to resolving
// user-defined functions from loaded files.
func (s *Server) lookupFunc(docURI protocol.DocumentURI, name string) (BuiltinFunc, bool) {
	if f, ok := s.catalog.Lookup(name); ok {
		return f, true
	}
	return s.resolveUserFunc(docURI, name)
}

// resolveUserFunc attempts to find a user-defined function by name in the
// current file or its loaded dependencies. Returns a BuiltinFunc with
// params extracted from the DefStmt, or false if not found.
func (s *Server) resolveUserFunc(docURI protocol.DocumentURI, name string) (BuiltinFunc, bool) {
	doc, ok := s.docs.Get(docURI)
	if !ok {
		return BuiltinFunc{}, false
	}

	filePath := uriToPath(docURI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return BuiltinFunc{}, false
	}

	// Check current file first.
	if bf, ok := defStmtToBuiltin(f, name); ok {
		return bf, true
	}

	// Check loaded files.
	for _, stmt := range f.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}
		for i, to := range load.To {
			exportedName := to.Name
			if i < len(load.From) {
				exportedName = load.From[i].Name
			}
			if to.Name != name {
				continue
			}
			resolved := s.resolveLoad(filePath, load.ModuleName())
			if resolved == "" {
				continue
			}
			data, err := os.ReadFile(resolved)
			if err != nil {
				continue
			}
			ext, _ := Parse(resolved, data)
			if ext == nil {
				continue
			}
			if bf, ok := defStmtToBuiltin(ext, exportedName); ok {
				bf.Name = name
				return bf, true
			}
		}
	}

	return BuiltinFunc{}, false
}

// defStmtToBuiltin extracts param metadata from a DefStmt.
func defStmtToBuiltin(f *syntax.File, name string) (BuiltinFunc, bool) {
	for _, stmt := range f.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if !ok || def.Name.Name != name {
			continue
		}

		var params []BuiltinParam
		for _, p := range def.Params {
			bp := paramFromExpr(p)
			if bp.Name != "" {
				params = append(params, bp)
			}
		}

		return BuiltinFunc{
			Name:   name,
			Params: params,
		}, true
	}
	return BuiltinFunc{}, false
}

func paramFromExpr(expr syntax.Expr) BuiltinParam {
	switch p := expr.(type) {
	case *syntax.Ident:
		return BuiltinParam{Name: p.Name, Required: true}
	case *syntax.BinaryExpr:
		// name=default
		if p.Op == syntax.EQ {
			if id, ok := p.X.(*syntax.Ident); ok {
				bp := BuiltinParam{Name: id.Name}
				if lit, ok := p.Y.(*syntax.Literal); ok {
					if s, ok := lit.Value.(string); ok {
						bp.Default = `"` + s + `"`
					} else {
						bp.Default = lit.Raw
					}
				}
				return bp
			}
		}
	case *syntax.UnaryExpr:
		// *args or **kwargs
		if id, ok := p.X.(*syntax.Ident); ok {
			return BuiltinParam{Name: p.Op.String() + id.Name}
		}
	}
	return BuiltinParam{}
}
