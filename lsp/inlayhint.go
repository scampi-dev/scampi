// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"encoding/json"
	"strings"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
)

// InlayHint types — not yet in go.lsp.dev/protocol.
// These mirror LSP 3.17 textDocument/inlayHint.

type InlayHintParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	Range        protocol.Range                  `json:"range"`
}

type InlayHint struct {
	Position protocol.Position `json:"position"`
	Label    string            `json:"label"`
	Kind     InlayHintKind     `json:"kind"`
}

type InlayHintKind int

const (
	InlayHintKindType      InlayHintKind = 1
	InlayHintKindParameter InlayHintKind = 2
)

// inlayHintHandler returns a jsonrpc2.Handler that intercepts
// textDocument/inlayHint and delegates everything else to next.
func (s *Server) inlayHintHandler(next jsonrpc2.Handler) jsonrpc2.Handler {
	return func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		switch req.Method() {
		case "textDocument/inlayHint":
			var params InlayHintParams
			if err := json.Unmarshal(req.Params(), &params); err != nil {
				return reply(ctx, nil, err)
			}
			hints := s.computeInlayHints(params)
			s.log.Printf("inlayHint: %s → %d hints", params.TextDocument.URI, len(hints))
			return reply(ctx, hints, nil)

		case "initialize":
			// Wrap the reply to inject inlayHintProvider into capabilities.
			wrappedReply := func(ctx context.Context, result any, err error) error {
				if err != nil {
					return reply(ctx, result, err)
				}
				// Re-serialize with the extra field.
				raw, marshalErr := json.Marshal(result)
				if marshalErr != nil {
					return reply(ctx, result, nil)
				}
				var m map[string]any
				if json.Unmarshal(raw, &m) == nil {
					if caps, ok := m["capabilities"].(map[string]any); ok {
						caps["inlayHintProvider"] = true
					}
				}
				return reply(ctx, m, nil)
			}
			return next(ctx, wrappedReply, req)

		default:
			return next(ctx, reply, req)
		}
	}
}

func (s *Server) computeInlayHints(params InlayHintParams) []InlayHint {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil
	}

	filePath := uriToPath(params.TextDocument.URI)
	c := tolerantCheck(filePath, []byte(doc.Content), s.modules)
	if c == nil {
		return nil
	}

	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil
	}

	var hints []InlayHint
	data := []byte(doc.Content)

	ast.Walk(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.LetDecl:
			// Type hint on let bindings: show inferred type after name.
			if n.Type == nil {
				if t := lookupLetType(c, n.Name.Name); t != nil {
					line, col := offsetToPos(data, int(n.Name.SrcSpan.End))
					hints = append(hints, InlayHint{
						Position: protocol.Position{Line: line, Character: col},
						Label:    ": " + t.String(),
						Kind:     InlayHintKindType,
					})
				}
			}
		case *ast.LetStmt:
			if n.Decl.Type == nil {
				if t := lookupLetType(c, n.Decl.Name.Name); t != nil {
					line, col := offsetToPos(data, int(n.Decl.Name.SrcSpan.End))
					hints = append(hints, InlayHint{
						Position: protocol.Position{Line: line, Character: col},
						Label:    ": " + t.String(),
						Kind:     InlayHintKindType,
					})
				}
			}
		case *ast.CallExpr:
			hints = append(hints, s.paramHints(c, n, data)...)
		case *ast.StructLit:
			hints = append(hints, s.structFieldHints(n, data)...)
		}
		return true
	}, nil)

	return hints
}

func (s *Server) paramHints(c *check.Checker, call *ast.CallExpr, src []byte) []InlayHint {
	// Resolve function name to get param names.
	funcName := exprFuncName(call.Fn)
	if funcName == "" {
		return nil
	}

	type paramDef struct {
		name string
		typ  string
	}

	// Collect param names + types from catalog or checker.
	var params []paramDef
	if f, ok := s.catalog.Lookup(funcName); ok {
		for _, p := range f.Params {
			params = append(params, paramDef{name: p.Name, typ: p.Type})
		}
	}
	if params == nil {
		if ft := lookupFuncType(c, funcName); ft != nil {
			for _, p := range ft.Params {
				typStr := ""
				if p.Type != nil {
					typStr = p.Type.String()
				}
				params = append(params, paramDef{name: p.Name, typ: typStr})
			}
		}
	}
	if params == nil {
		return nil
	}

	// Build a name→type map for kwarg type hints.
	paramTypes := make(map[string]string, len(params))
	for _, p := range params {
		paramTypes[p.name] = p.typ
	}

	var hints []InlayHint
	for i, arg := range call.Args {
		if arg.Name != nil {
			// Kwarg: show type hint after the kwarg name.
			if typ, ok := paramTypes[arg.Name.Name]; ok && typ != "" {
				line, col := offsetToPos(src, int(arg.Name.SrcSpan.End))
				hints = append(hints, InlayHint{
					Position: protocol.Position{Line: line, Character: col},
					Label:    ": " + typ,
					Kind:     InlayHintKindType,
				})
			}
		} else {
			// Positional: show param name hint before the value.
			if i >= len(params) {
				break
			}
			line, col := offsetToPos(src, int(arg.Value.Span().Start))
			hints = append(hints, InlayHint{
				Position: protocol.Position{Line: line, Character: col},
				Label:    params[i].name + ":",
				Kind:     InlayHintKindParameter,
			})
		}
	}
	return hints
}

func exprFuncName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return id.Name + "." + e.Sel.Name
		}
	}
	return ""
}

func (s *Server) structFieldHints(lit *ast.StructLit, src []byte) []InlayHint {
	if lit.Type == nil || len(lit.Fields) == 0 {
		return nil
	}

	// Resolve the struct lit's type name to get field types from catalog.
	typeName := typeExprFuncName(lit.Type)
	if typeName == "" {
		return nil
	}

	paramTypes := make(map[string]string)
	if f, ok := s.catalog.Lookup(typeName); ok {
		for _, p := range f.Params {
			paramTypes[p.Name] = p.Type
		}
	}
	if len(paramTypes) == 0 {
		return nil
	}

	var hints []InlayHint
	for _, fi := range lit.Fields {
		typ, ok := paramTypes[fi.Name.Name]
		if !ok || typ == "" {
			continue
		}
		line, col := offsetToPos(src, int(fi.Name.SrcSpan.End))
		hints = append(hints, InlayHint{
			Position: protocol.Position{Line: line, Character: col},
			Label:    ": " + typ,
			Kind:     InlayHintKindType,
		})
	}
	return hints
}

func typeExprFuncName(te ast.TypeExpr) string {
	switch t := te.(type) {
	case *ast.NamedType:
		parts := make([]string, len(t.Name.Parts))
		for i, p := range t.Name.Parts {
			parts[i] = p.Name
		}
		return strings.Join(parts, ".")
	}
	return ""
}

func lookupFuncType(c *check.Checker, name string) *check.FuncType {
	bindings := c.AllBindings()
	if sym, ok := bindings[name]; ok {
		if ft, ok := sym.Type.(*check.FuncType); ok {
			return ft
		}
	}
	// Try file scope for top-level funcs.
	scope := c.FileScope()
	if scope == nil {
		return nil
	}
	sym := scope.Lookup(name)
	if sym == nil {
		return nil
	}
	ft, _ := sym.Type.(*check.FuncType)
	return ft
}

func lookupLetType(c *check.Checker, name string) check.Type {
	if sym, ok := c.AllBindings()[name]; ok {
		return sym.Type
	}
	return nil
}

func offsetToPos(src []byte, offset int) (line, col uint32) {
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return line, col
}
