// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/spec"
)

// runAttributeStaticChecks walks the AST looking for func/UFCS call
// sites whose parameters carry `@`-attributes and dispatches the
// registered AttributeBehaviour for each. This path covers shapes
// where eval consumed the call's args (`secrets.get("key")`, plain
// `f(arg)`, `std.deploy(name=, targets=)`) — the literal lives only
// in the AST.
//
// Decl/struct-literal invocations (`pve.lxc { id = ... }`) are NOT
// validated here; they go through runAttributeEvalChecks which sees
// the eval-resolved values. That gets us comprehensions, let-bindings,
// and field access for free.
//
// Non-literal call args (let-bound, computed, etc.) skip silently —
// behaviours that need a literal (like @secretkey) bail early on
// their own.
func runAttributeStaticChecks(
	f *ast.File,
	source []byte,
	cfgPath string,
	fileScope *check.Scope,
	modules map[string]*check.Scope,
	registry *AttributeRegistry,
	result *eval.Result,
) error {
	if registry == nil {
		return nil
	}
	ctx := &linkContext{}
	visitor := &attributeCheckVisitor{
		ctx:       ctx,
		registry:  registry,
		fileScope: fileScope,
		modules:   modules,
		source:    source,
		cfgPath:   cfgPath,
		result:    result,
	}
	ast.Walk(f, visitor.enter, nil)
	if len(ctx.diags) == 0 {
		return nil
	}
	return ctx.diags
}

// attributeCheckVisitor walks the AST and dispatches attribute
// behaviours for each annotated call site.
type attributeCheckVisitor struct {
	ctx       *linkContext
	registry  *AttributeRegistry
	fileScope *check.Scope
	modules   map[string]*check.Scope
	source    []byte
	cfgPath   string
	result    *eval.Result
}

// enter dispatches attribute checks for AST shapes whose call-site
// arguments don't survive eval — UFCS and plain function calls.
// Decl/struct-literal invocations are validated via the eval-walker
// (see attribute_eval_walk.go) which sees the resolved values rather
// than the syntactic AST.
func (v *attributeCheckVisitor) enter(n ast.Node) bool {
	if n, ok := n.(*ast.CallExpr); ok {
		if n.UFCS && n.UFCSModule != "" {
			if ft := v.resolveUFCSTarget(n); ft != nil {
				v.checkCall(n, ft)
			}
		} else if ft := v.resolveCallTarget(n.Fn); ft != nil {
			v.checkCall(n, ft)
		}
	}
	return true
}

// resolveUFCSTarget resolves the function type for a UFCS call.
// The real function lives in call.UFCSModule, not the receiver.
func (v *attributeCheckVisitor) resolveUFCSTarget(call *ast.CallExpr) *check.FuncType {
	mod := v.modules[call.UFCSModule]
	if mod == nil {
		return nil
	}
	sel, ok := call.Fn.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	sym := mod.Lookup(sel.Sel.Name)
	if sym == nil {
		return nil
	}
	ft, _ := sym.Type.(*check.FuncType)
	return ft
}

// attrDoc returns the doc-comment block of an attribute's resolved
// type, or empty if resolution failed at check time.
func attrDoc(attr check.ResolvedAttribute) string {
	if attr.Type == nil {
		return ""
	}
	return attr.Type.Doc
}

// resolveCallTarget walks the call's function expression and tries to
// look up the resulting symbol's FuncType. Currently handles two
// shapes: a bare identifier (resolves in the file scope) and a
// dotted name like `secrets.from_age` (resolves in the imported
// module's scope). Returns nil if the function isn't a known
// annotated target.
func (v *attributeCheckVisitor) resolveCallTarget(e ast.Expr) *check.FuncType {
	switch fn := e.(type) {
	case *ast.Ident:
		if v.fileScope == nil {
			return nil
		}
		sym := v.fileScope.Lookup(fn.Name)
		if sym == nil {
			return nil
		}
		ft, _ := sym.Type.(*check.FuncType)
		return ft
	case *ast.SelectorExpr:
		modIdent, ok := fn.X.(*ast.Ident)
		if !ok {
			return nil
		}
		mod := v.modules[modIdent.Name]
		if mod == nil {
			return nil
		}
		sym := mod.Lookup(fn.Sel.Name)
		if sym == nil {
			return nil
		}
		ft, _ := sym.Type.(*check.FuncType)
		return ft
	}
	return nil
}

// checkCall iterates the parameters of the resolved function and, for
// each parameter that carries an attribute, dispatches the registered
// behaviour with the corresponding argument expression.
func (v *attributeCheckVisitor) checkCall(call *ast.CallExpr, ft *check.FuncType) {
	if len(ft.Params) == 0 {
		return
	}
	argFor := bindCallArgs(call, ft)

	// For UFCS calls, the receiver is param 0 but isn't in Args —
	// it lives in call.Fn.(*SelectorExpr).X. Shift all arg indices
	// up by 1 so they align with the function's parameter list.
	if call.UFCS {
		shifted := make(map[int]ast.Expr, len(argFor))
		for i, e := range argFor {
			shifted[i+1] = e
		}
		argFor = shifted
	}

	// If this is a UFCS call, try to extract the resolver backend
	// from the receiver for @secretkey validation.
	var resolverBackend ResolverBackendFunc
	if call.UFCS && v.result != nil {
		resolverBackend = v.resolverBackendFromResult
	}

	for i, p := range ft.Params {
		if len(p.Attributes) == 0 {
			continue
		}
		argExpr, ok := argFor[i]
		if !ok {
			continue
		}
		// Non-literal args (let-bound, computed, etc.) skip silently
		// — eval has consumed the value, so there's nothing to
		// validate statically. Behaviours that absolutely need a
		// literal (@secretkey) bail early on their own.
		for _, attr := range p.Attributes {
			behaviour := v.registry.Lookup(attr.QualifiedName)
			if behaviour == nil {
				continue
			}
			useSpan := nodeSourceSpan(argExpr, v.source, v.cfgPath)
			ctx := StaticCheckContext{
				Linker:    v.ctx,
				AttrName:  attr.QualifiedName,
				AttrArgs:  attr.Args,
				AttrDoc:   attrDoc(attr),
				ParamName: p.Name,
				ParamArg:  argExpr,
				UseSpan:   useSpan,
			}
			// For UFCS calls with a resolver receiver, extract
			// the backend from the let-bound variable.
			if resolverBackend != nil {
				if sel, ok := call.Fn.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok {
						ctx.ResolverBackend = resolverBackend(ident.Name)
					}
				}
			}
			behaviour.StaticCheck(ctx)
		}
	}
}

// resolverBackendFromResult looks up a let-bound variable name in the
// eval result and returns its secret.Backend if it's an OpaqueVal
// wrapping a secret backend.
func (v *attributeCheckVisitor) resolverBackendFromResult(name string) secret.Backend {
	if v.result == nil || v.result.Bindings == nil {
		return nil
	}
	val, ok := v.result.Bindings[name]
	if !ok {
		return nil
	}
	opaque, ok := val.(*eval.OpaqueVal)
	if !ok {
		return nil
	}
	b, _ := opaque.Inner.(secret.Backend)
	return b
}

// bindCallArgs maps parameter indices to argument expressions for a
// CallExpr. Positional arguments bind to the leading parameters in
// order; keyword arguments bind by name. Parameters with no
// corresponding argument (e.g. optionals using their default) are
// absent from the result.
func bindCallArgs(call *ast.CallExpr, ft *check.FuncType) map[int]ast.Expr {
	out := make(map[int]ast.Expr, len(call.Args))
	posIdx := 0
	for _, a := range call.Args {
		if a.Name == nil {
			if posIdx < len(ft.Params) {
				out[posIdx] = a.Value
			}
			posIdx++
			continue
		}
		for i, p := range ft.Params {
			if p.Name == a.Name.Name {
				out[i] = a.Value
				break
			}
		}
	}
	return out
}

// linkContext is the linker-side LinkContext implementation passed
// to AttributeBehaviour.StaticCheck. It collects diagnostics emitted
// during the static check pass; the caller wraps them into a single
// diagnostic.Diagnostics for return through the standard pipeline.
type linkContext struct {
	diags diagnostic.Diagnostics
}

func (lc *linkContext) Emit(d diagnostic.Diagnostic) {
	lc.diags = append(lc.diags, d)
}

func nodeSourceSpan(node ast.Node, source []byte, cfgPath string) spec.SourceSpan {
	span := node.Span()
	startLine, startCol := offsetToLineCol(source, int(span.Start))
	endLine, endCol := offsetToLineCol(source, int(span.End))
	return spec.SourceSpan{
		Filename:  cfgPath,
		StartLine: startLine,
		StartCol:  startCol,
		EndLine:   endLine,
		EndCol:    endCol,
	}
}
