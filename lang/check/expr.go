// SPDX-License-Identifier: GPL-3.0-only

package check

import (
	"strconv"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/token"
)

// typeOf computes the type of an expression. Returns nil on error
// (after recording a diagnostic). This is recursive and bottom-up:
// children are resolved before parents.
func (c *Checker) typeOf(e ast.Expr) Type {
	if e == nil {
		return nil
	}
	switch e := e.(type) {
	case *ast.IntLit:
		return IntType
	case *ast.BoolLit:
		return BoolType
	case *ast.NoneLit:
		return NoneType
	case *ast.SelfLit:
		return c.selfType()
	case *ast.StringLit:
		c.checkStringParts(e)
		return StringType
	case *ast.Ident:
		return c.resolveIdent(e)
	case *ast.SelectorExpr:
		return c.resolveSelector(e)
	case *ast.CallExpr:
		return c.checkCall(e)
	case *ast.StructLit:
		return c.checkStructLit(e)
	case *ast.ListLit:
		return c.checkListLit(e)
	case *ast.MapLit:
		return c.checkMapLit(e)
	case *ast.IndexExpr:
		return c.checkIndex(e)
	case *ast.BinaryExpr:
		return c.checkBinary(e)
	case *ast.UnaryExpr:
		return c.checkUnary(e)
	case *ast.IfExpr:
		return c.checkIfExpr(e)
	case *ast.ListComp:
		return c.checkListComp(e)
	case *ast.DottedName:
		return c.resolveDottedName(e)
	}
	c.errAt(e.Span(), "cannot determine type of expression")
	return nil
}

// Ident resolution
// -----------------------------------------------------------------------------

func (c *Checker) resolveIdent(id *ast.Ident) Type {
	sym := c.scope.Lookup(id.Name)
	if sym == nil {
		c.errAt(id.SrcSpan, "undefined: "+id.Name)
		return nil
	}
	return sym.Type
}

func (c *Checker) resolveDottedName(dn *ast.DottedName) Type {
	if len(dn.Parts) == 0 {
		return nil
	}
	sym := c.scope.Lookup(dn.Parts[0].Name)
	if sym == nil {
		c.errAt(dn.Parts[0].SrcSpan, "undefined: "+dn.Parts[0].Name)
		return nil
	}
	if len(dn.Parts) == 1 {
		return sym.Type
	}
	// Multi-part: first part must be a module import.
	if sym.Kind != SymImport {
		c.errAt(dn.SrcSpan, dn.Parts[0].Name+" is not a module")
		return nil
	}
	return c.resolveModuleMember(dn.Parts[0].Name, dn.Parts[1:], dn.SrcSpan)
}

// Selector resolution (x.y)
// -----------------------------------------------------------------------------

func (c *Checker) resolveSelector(sel *ast.SelectorExpr) Type {
	xType := c.typeOf(sel.X)
	if xType == nil {
		return nil
	}
	name := sel.Sel.Name

	// Module namespace member access (import.member).
	if id, ok := sel.X.(*ast.Ident); ok {
		sym := c.scope.Lookup(id.Name)
		if sym != nil && sym.Kind == SymImport {
			return c.resolveModuleMember(id.Name, []*ast.Ident{sel.Sel}, sel.SrcSpan)
		}
	}

	// Struct field access.
	if st, ok := xType.(*StructType); ok {
		for _, f := range st.Fields {
			if f.Name == name {
				return f.Type
			}
		}
		c.errAt(sel.Sel.SrcSpan, "no field "+name+" on "+st.Name)
		return nil
	}

	// Enum variant access (EnumType.variant).
	if et, ok := xType.(*EnumType); ok {
		if et.HasVariant(name) {
			return et
		}
		c.errAt(sel.Sel.SrcSpan, "no variant "+name+" on enum "+et.Name)
		return nil
	}

	c.errAt(sel.SrcSpan, "cannot access ."+name+" on "+xType.String())
	return nil
}

func (c *Checker) resolveModuleMember(modName string, parts []*ast.Ident, span token.Span) Type {
	mod, ok := c.modules[modName]
	if !ok {
		c.errAt(span, "unknown module: "+modName)
		return nil
	}
	// Walk the dotted chain through the module scope.
	// For now: look up the first remaining part directly.
	sym := mod.Lookup(parts[0].Name)
	if sym == nil {
		c.errAt(parts[0].SrcSpan, modName+"."+parts[0].Name+": undefined")
		return nil
	}
	if len(parts) == 1 {
		return sym.Type
	}
	// Deeper access (e.g. std.PkgState.present → enum variant).
	return c.chainAccess(sym.Type, parts[1:], span)
}

func (c *Checker) chainAccess(t Type, parts []*ast.Ident, span token.Span) Type {
	for _, part := range parts {
		if t == nil {
			return nil
		}
		switch tt := t.(type) {
		case *EnumType:
			if tt.HasVariant(part.Name) {
				return tt
			}
			c.errAt(part.SrcSpan, "no variant "+part.Name+" on enum "+tt.Name)
			return nil
		case *StructType:
			found := false
			for _, f := range tt.Fields {
				if f.Name == part.Name {
					t = f.Type
					found = true
					break
				}
			}
			if !found {
				c.errAt(part.SrcSpan, "no field "+part.Name+" on "+tt.Name)
				return nil
			}
		default:
			c.errAt(span, "cannot access ."+part.Name+" on "+t.String())
			return nil
		}
	}
	return t
}

// Call checking
// -----------------------------------------------------------------------------

func (c *Checker) checkCall(call *ast.CallExpr) Type {
	fnType := c.typeOf(call.Fn)
	if fnType == nil {
		return nil
	}
	ft, ok := fnType.(*FuncType)
	if !ok {
		c.errAt(call.SrcSpan, "cannot call "+fnType.String())
		return nil
	}
	minArgs := 0
	for _, p := range ft.Params {
		if !p.HasDef {
			minArgs++
		}
	}
	if len(call.Args) < minArgs {
		c.errAt(call.SrcSpan, "too few arguments")
	}
	if len(call.Args) > len(ft.Params) {
		c.errAt(call.SrcSpan, "too many arguments")
	}
	for i, arg := range call.Args {
		argT := c.typeOf(arg.Value)
		if argT == nil {
			continue
		}
		// For keyword args, find the param by name.
		var paramT Type
		if arg.Name != nil {
			for _, p := range ft.Params {
				if p.Name == arg.Name.Name {
					paramT = p.Type
					break
				}
			}
		} else if i < len(ft.Params) {
			paramT = ft.Params[i].Type
		}
		if paramT != nil && !IsAssignableTo(argT, paramT) {
			c.errAt(arg.Value.Span(), "argument type mismatch: got "+argT.String()+", want "+paramT.String())
		}
	}
	return ft.Ret
}

// Struct/step literal checking
// -----------------------------------------------------------------------------

func (c *Checker) checkStructLit(lit *ast.StructLit) Type {
	if lit.Type == nil {
		// Inferred struct lit: { field = value }. Type determined by
		// context (the expected type from the enclosing field/param).
		// For now, treat as map[string, any].
		for _, f := range lit.Fields {
			c.typeOf(f.Value)
		}
		return &Map{Key: StringType, Value: AnyType}
	}
	t := c.resolveType(lit.Type)
	if t == nil {
		return nil
	}
	switch tt := t.(type) {
	case *OpaqueType:
		c.errAt(lit.SrcSpan, "cannot construct opaque type "+tt.Name)
		return nil
	case *StructType:
		c.checkFieldInits(tt.Fields, lit.Fields, tt.Name, lit.SrcSpan)
		return tt
	case *DeclType:
		c.checkFieldInits(tt.Params, lit.Fields, tt.Name, lit.SrcSpan)
		// Check body statements (step invocations inside deploy/step bodies).
		for _, s := range lit.Body {
			ast.Walk(s, c.enter, c.leave)
		}
		return tt.Ret
	}
	c.errAt(lit.SrcSpan, t.String()+" is not a struct or decl type")
	return nil
}

func (c *Checker) checkFieldInits(
	decl []*FieldDef,
	inits []*ast.FieldInit,
	typeName string,
	span token.Span,
) {
	declared := make(map[string]*FieldDef, len(decl))
	for _, d := range decl {
		declared[d.Name] = d
	}
	seen := make(map[string]bool, len(inits))
	for _, f := range inits {
		name := f.Name.Name
		if seen[name] {
			c.errAt(f.SrcSpan, "duplicate field: "+name)
			continue
		}
		seen[name] = true
		d, ok := declared[name]
		if !ok {
			c.errAt(f.Name.SrcSpan, "unknown field "+name+" on "+typeName)
			continue
		}
		vt := c.typeOf(f.Value)
		if vt != nil && d.Type != nil && !IsAssignableTo(vt, d.Type) {
			c.errAt(
				f.SrcSpan,
				"field "+name+": got "+vt.String()+", want "+d.Type.String(),
			)
		}
	}
	// Check required fields are present. Optional types (T?) implicitly
	// default to none, so they're never required.
	for _, d := range decl {
		if !d.HasDef && !isOptional(d.Type) && !seen[d.Name] {
			c.errAt(span, "missing required field: "+d.Name)
		}
	}
}

func isOptional(t Type) bool {
	_, ok := t.(*Optional)
	return ok
}

// Collection literals
// -----------------------------------------------------------------------------

func (c *Checker) checkListLit(lit *ast.ListLit) Type {
	if len(lit.Items) == 0 {
		return &List{Elem: AnyType}
	}
	var elem Type
	for _, item := range lit.Items {
		t := c.typeOf(item)
		if t == nil {
			continue
		}
		if elem == nil {
			elem = t
		}
		// Don't enforce homogeneity for now; evaluator handles mixed lists
		// via any. A more refined check can come later.
	}
	if elem == nil {
		elem = AnyType
	}
	return &List{Elem: elem}
}

func (c *Checker) checkMapLit(lit *ast.MapLit) Type {
	var valType Type
	for _, e := range lit.Entries {
		c.typeOf(e.Key)
		vt := c.typeOf(e.Value)
		if vt == nil {
			continue
		}
		if valType == nil {
			valType = vt
		} else if valType != vt {
			valType = AnyType
		}
	}
	if valType == nil {
		valType = AnyType
	}
	return &Map{Key: StringType, Value: valType}
}

// Index expression
// -----------------------------------------------------------------------------

func (c *Checker) checkIndex(idx *ast.IndexExpr) Type {
	xType := c.typeOf(idx.X)
	idxType := c.typeOf(idx.Index)
	if xType == nil {
		return nil
	}
	// Unwrap optional: T?[k] → T[k] (user is expected to nil-check first).
	if opt, ok := xType.(*Optional); ok {
		xType = opt.Inner
	}
	switch t := xType.(type) {
	case *List:
		if idxType != nil && idxType != IntType {
			c.errAt(idx.Index.Span(), "list index must be int, got "+idxType.String())
		}
		return t.Elem
	case *Map:
		if idxType != nil && !IsAssignableTo(idxType, t.Key) {
			c.errAt(idx.Index.Span(), "map key type mismatch")
		}
		return t.Value
	}
	c.errAt(idx.SrcSpan, "cannot index "+xType.String())
	return nil
}

// Binary + unary operators
// -----------------------------------------------------------------------------

func (c *Checker) checkBinary(bin *ast.BinaryExpr) Type {
	lt := c.typeOf(bin.Left)
	rt := c.typeOf(bin.Right)

	switch bin.Op {
	case token.Plus:
		if lt == StringType && rt == StringType {
			return StringType
		}
		if lt == IntType && rt == IntType {
			return IntType
		}
		if ll, ok := lt.(*List); ok {
			if _, ok := rt.(*List); ok {
				return ll
			}
		}
		if lt != nil && rt != nil {
			c.errAt(bin.SrcSpan, "cannot add "+lt.String()+" and "+rt.String())
		}
		return nil
	case token.Minus, token.Star, token.Slash, token.Percent:
		if lt == IntType && rt == IntType {
			return IntType
		}
		if lt != nil && rt != nil {
			c.errAt(bin.SrcSpan, "arithmetic requires int operands")
		}
		return nil
	case token.Eq, token.Neq, token.Lt, token.Gt, token.Leq, token.Geq:
		return BoolType
	case token.And, token.Or:
		if lt != nil && lt != BoolType {
			c.errAt(bin.Left.Span(), "&& and || require bool operands, got "+lt.String())
		}
		if rt != nil && rt != BoolType {
			c.errAt(bin.Right.Span(), "&& and || require bool operands, got "+rt.String())
		}
		return BoolType
	case token.In:
		return BoolType
	}
	return nil
}

func (c *Checker) checkUnary(un *ast.UnaryExpr) Type {
	xt := c.typeOf(un.X)
	switch un.Op {
	case token.Not:
		if xt != nil && xt != BoolType {
			c.errAt(un.SrcSpan, "! requires bool operand, got "+xt.String())
		}
		return BoolType
	case token.Minus:
		if xt != nil && xt != IntType {
			c.errAt(un.SrcSpan, "unary minus requires int operand, got "+xt.String())
		}
		return IntType
	}
	return nil
}

// If expression
// -----------------------------------------------------------------------------

func (c *Checker) checkIfExpr(ife *ast.IfExpr) Type {
	ct := c.typeOf(ife.Cond)
	if ct != nil && ct != BoolType {
		c.errAt(ife.Cond.Span(), "if condition must be bool, got "+ct.String())
	}
	tt := c.typeOf(ife.Then)
	c.typeOf(ife.Else)
	return tt
}

// List comprehension
// -----------------------------------------------------------------------------

func (c *Checker) checkListComp(comp *ast.ListComp) Type {
	iterT := c.typeOf(comp.Iter)
	if iterT != nil {
		if lt, ok := iterT.(*List); ok {
			// Bind loop variable in a temporary scope.
			c.pushScope(ScopeBlock)
			c.scope.Define(&Symbol{
				Name: comp.Var.Name,
				Type: lt.Elem,
				Kind: SymLet,
				Span: comp.Var.SrcSpan,
			})
			elemT := c.typeOf(comp.Expr)
			if comp.Cond != nil {
				c.typeOf(comp.Cond)
			}
			c.popScope()
			if elemT != nil {
				return &List{Elem: elemT}
			}
		}
	}
	return &List{Elem: AnyType}
}

// String interpolation
// -----------------------------------------------------------------------------

func (c *Checker) checkStringParts(lit *ast.StringLit) {
	for _, p := range lit.Parts {
		if interp, ok := p.(*ast.StringInterp); ok {
			c.typeOf(interp.Expr)
		}
	}
}

// Self type
// -----------------------------------------------------------------------------

func (c *Checker) selfType() Type {
	if c.selfFields == nil {
		c.errAt(token.Span{}, "self used outside of step body")
		return nil
	}
	return &StructType{Name: "self", Fields: c.selfFields}
}

// Int parsing (for IntLit value resolution)
// -----------------------------------------------------------------------------

// ParseInt resolves the raw text of an IntLit to a value. Exported so
// the evaluator can reuse it without re-parsing.
func ParseInt(raw string) (int64, error) {
	if len(raw) > 2 {
		switch raw[:2] {
		case "0x", "0X":
			return strconv.ParseInt(raw[2:], 16, 64)
		case "0b", "0B":
			return strconv.ParseInt(raw[2:], 2, 64)
		case "0o", "0O":
			return strconv.ParseInt(raw[2:], 8, 64)
		}
	}
	return strconv.ParseInt(raw, 10, 64)
}
