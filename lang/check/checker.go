// SPDX-License-Identifier: GPL-3.0-only

package check

import (
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/token"
)

// Checker walks a parsed AST via ast.Walk and performs type checking.
// After Check() returns, callers should inspect Errors().
type Checker struct {
	scope *Scope
	errs  []Error

	// modules maps import leaf-names to their scopes.
	modules map[string]*Scope
}

// Error is a type-checker error.
type Error struct {
	Span token.Span
	Msg  string
}

func (e Error) Error() string { return e.Msg }

// New creates a checker with the std module pre-loaded.
func New() *Checker {
	return &Checker{
		modules: map[string]*Scope{
			"std": StdModule(),
		},
	}
}

// Errors returns accumulated checker errors.
func (c *Checker) Errors() []Error { return c.errs }

// Check type-checks a parsed file using ast.Walk for traversal.
func (c *Checker) Check(f *ast.File) {
	c.scope = NewScope(nil, ScopeFile)

	// Register imports first (they affect all subsequent resolution).
	for _, imp := range f.Imports {
		c.checkImport(imp)
	}

	// Forward-declare all top-level names so order doesn't matter.
	for _, d := range f.Decls {
		c.registerDecl(d)
	}

	// Walk the full AST for checking.
	ast.Walk(f, c.enter, c.leave)
}

func (c *Checker) errAt(span token.Span, msg string) {
	c.errs = append(c.errs, Error{Span: span, Msg: msg})
}

// enter is the pre-visit callback for ast.Walk.
func (c *Checker) enter(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.File:
		// Already handled imports and forward-decls above.
		return true

	case *ast.StructDecl:
		c.checkStructDecl(n)
		return false // children already visited by checkStructDecl

	case *ast.EnumDecl:
		return false // fully registered in forward pass

	case *ast.FuncDecl:
		c.checkFuncDecl(n)
		return false // we handle our own child walk

	case *ast.StepDecl:
		c.checkStepDecl(n)
		return false

	case *ast.LetDecl:
		c.checkLetDecl(n)
		return false

	case *ast.ForStmt:
		c.pushScope(ScopeBlock)
		return true // let Walk visit children

	case *ast.IfStmt:
		c.pushScope(ScopeBlock)
		return true

	case *ast.AssignStmt:
		if !c.scope.AllowsMutation() {
			c.errAt(n.SrcSpan, "mutation not allowed outside func bodies")
		}
		return true
	}
	return true
}

// leave is the post-visit callback for ast.Walk.
func (c *Checker) leave(n ast.Node) {
	switch n.(type) {
	case *ast.ForStmt, *ast.IfStmt:
		c.popScope()
	}
}

func (c *Checker) pushScope(kind ScopeKind) {
	c.scope = NewScope(c.scope, kind)
}

func (c *Checker) popScope() {
	if c.scope.parent != nil {
		c.scope = c.scope.parent
	}
}

// Import resolution
// -----------------------------------------------------------------------------

func (c *Checker) checkImport(imp *ast.ImportDecl) {
	leaf := importLeaf(imp.Path)
	_, ok := c.modules[leaf]
	if !ok {
		c.errAt(imp.SrcSpan, "unknown module: "+imp.Path)
		return
	}
	if !c.scope.Define(&Symbol{
		Name: leaf,
		Type: nil,
		Kind: SymImport,
		Span: imp.SrcSpan,
	}) {
		c.errAt(imp.SrcSpan, "duplicate import: "+leaf)
	}
}

func importLeaf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// Forward declaration registration
// -----------------------------------------------------------------------------

func (c *Checker) registerDecl(d ast.Decl) {
	switch d := d.(type) {
	case *ast.StructDecl:
		st := &StructType{Name: d.Name.Name}
		c.scope.Define(&Symbol{
			Name: d.Name.Name, Type: st, Kind: SymStruct, Span: d.SrcSpan,
		})
	case *ast.EnumDecl:
		var variants []string
		for _, v := range d.Variants {
			variants = append(variants, v.Name)
		}
		c.scope.Define(&Symbol{
			Name: d.Name.Name,
			Type: &EnumType{Name: d.Name.Name, Variants: variants},
			Kind: SymEnum, Span: d.SrcSpan,
		})
	case *ast.FuncDecl:
		c.scope.Define(&Symbol{
			Name: d.Name.Name, Kind: SymFunc, Span: d.SrcSpan,
		})
	case *ast.StepDecl:
		c.scope.Define(&Symbol{
			Name: d.Name.Parts[0].Name, Kind: SymStep, Span: d.SrcSpan,
		})
	case *ast.LetDecl:
		c.scope.Define(&Symbol{
			Name: d.Name.Name, Kind: SymLet, Span: d.SrcSpan,
		})
	}
}

// Declaration checking
// -----------------------------------------------------------------------------

func (c *Checker) checkStructDecl(d *ast.StructDecl) {
	sym := c.scope.Lookup(d.Name.Name)
	if sym == nil {
		return
	}
	st, ok := sym.Type.(*StructType)
	if !ok {
		return
	}
	for _, f := range d.Fields {
		ft := c.resolveType(f.Type)
		if ft == nil {
			c.errAt(f.SrcSpan, "unknown type in field "+f.Name.Name)
			continue
		}
		st.Fields = append(st.Fields, &FieldDef{
			Name:   f.Name.Name,
			Type:   ft,
			HasDef: f.Default != nil,
		})
	}
}

func (c *Checker) checkFuncDecl(d *ast.FuncDecl) {
	var ret Type
	if d.Ret != nil {
		ret = c.resolveType(d.Ret)
	}
	sym := c.scope.Lookup(d.Name.Name)
	if sym != nil {
		var params []*FieldDef
		for _, p := range d.Params {
			pt := c.resolveType(p.Type)
			params = append(params, &FieldDef{
				Name: p.Name.Name, Type: pt, HasDef: p.Default != nil,
			})
		}
		sym.Type = &FuncType{Params: params, Ret: ret}
	}
	if d.Body != nil {
		c.pushScope(ScopeFunc)
		// Register params in the func scope.
		for _, p := range d.Params {
			pt := c.resolveType(p.Type)
			c.scope.Define(&Symbol{
				Name: p.Name.Name, Type: pt, Kind: SymParam, Span: p.SrcSpan,
			})
		}
		// Walk the body.
		ast.Walk(d.Body, c.enter, c.leave)
		c.popScope()
	}
}

func (c *Checker) checkStepDecl(d *ast.StepDecl) {
	var ret Type = StepInstanceType
	if d.Ret != nil {
		ret = c.resolveType(d.Ret)
	}
	name := d.Name.Parts[0].Name
	sym := c.scope.Lookup(name)
	if sym != nil {
		var params []*FieldDef
		for _, p := range d.Params {
			pt := c.resolveType(p.Type)
			params = append(params, &FieldDef{
				Name: p.Name.Name, Type: pt, HasDef: p.Default != nil,
			})
		}
		sym.Type = &StepType{
			Name: name, Params: params, Ret: ret, HasBody: d.Body != nil,
		}
	}
	if d.Body != nil {
		c.pushScope(ScopeStep)
		for _, p := range d.Params {
			pt := c.resolveType(p.Type)
			c.scope.Define(&Symbol{
				Name: p.Name.Name, Type: pt, Kind: SymParam, Span: p.SrcSpan,
			})
		}
		ast.Walk(d.Body, c.enter, c.leave)
		c.popScope()
	}
}

func (c *Checker) checkLetDecl(d *ast.LetDecl) {
	// TODO: infer type from value expression once expr checking lands.
	_ = d
}

// Type resolution
// -----------------------------------------------------------------------------

func (c *Checker) resolveType(t ast.TypeExpr) Type {
	if t == nil {
		return nil
	}
	switch t := t.(type) {
	case *ast.NamedType:
		return c.resolveNamedType(t)
	case *ast.GenericType:
		return c.resolveGenericType(t)
	case *ast.OptionalType:
		inner := c.resolveType(t.Inner)
		if inner == nil {
			return nil
		}
		return &Optional{Inner: inner}
	}
	return nil
}

func (c *Checker) resolveNamedType(t *ast.NamedType) Type {
	if len(t.Name.Parts) == 1 {
		name := t.Name.Parts[0].Name
		if bt := builtinByName(name); bt != nil {
			return bt
		}
		sym := c.scope.Lookup(name)
		if sym != nil && (sym.Kind == SymStruct || sym.Kind == SymEnum) {
			return sym.Type
		}
		c.errAt(t.SrcSpan, "unknown type: "+name)
		return nil
	}
	c.errAt(t.SrcSpan, "qualified type names not yet supported")
	return nil
}

func builtinByName(name string) Type {
	switch name {
	case "string":
		return StringType
	case "int":
		return IntType
	case "bool":
		return BoolType
	case "any":
		return AnyType
	case "StepInstance":
		return StepInstanceType
	case "Target":
		return TargetType
	case "Deploy":
		return DeployType
	case "SecretsConfig":
		return SecretsConfigType
	case "Source":
		return SourceType
	case "PkgSource":
		return PkgSourceType
	case "Auth":
		return AuthType
	case "TLS":
		return TLSType
	case "Body":
		return BodyType
	case "Check":
		return CheckType
	}
	return nil
}

func (c *Checker) resolveGenericType(t *ast.GenericType) Type {
	switch t.Name.Name {
	case "list":
		if len(t.Args) != 1 {
			c.errAt(t.SrcSpan, "list takes exactly 1 type argument")
			return nil
		}
		elem := c.resolveType(t.Args[0])
		if elem == nil {
			return nil
		}
		return &List{Elem: elem}
	case "map":
		if len(t.Args) != 2 {
			c.errAt(t.SrcSpan, "map takes exactly 2 type arguments")
			return nil
		}
		k := c.resolveType(t.Args[0])
		v := c.resolveType(t.Args[1])
		if k == nil || v == nil {
			return nil
		}
		return &Map{Key: k, Value: v}
	}
	c.errAt(t.SrcSpan, "unknown generic type: "+t.Name.Name)
	return nil
}
