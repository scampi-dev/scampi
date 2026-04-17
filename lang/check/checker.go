// SPDX-License-Identifier: GPL-3.0-only

package check

import (
	"strings"

	"scampi.dev/scampi/errs"
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

	// selfFields is set when inside a step body to the step's params.
	selfFields []*FieldDef

	// returnType is the expected return type of the enclosing func.
	// nil when not inside a func body.
	returnType Type

	// modName is the name of the module being checked (set from
	// f.Module at the start of Check). Used to qualify single-segment
	// attribute references on Field.Attributes — for example, an
	// `@nonempty` annotation on a field declared in `module std`
	// resolves to the qualified name `std.@nonempty`.
	modName string

	// allBindings is a flat fallback map of every value-shaped
	// symbol the checker has resolved during the walk (lets,
	// params, for-loop vars), regardless of scope nesting. Tools
	// that operate after the walk (LSP completion, hover) can look
	// up nested bindings here even though their original scopes
	// have been popped from c.scope. Last-write-wins on shadowing
	// — sufficient for completion, not enough for any feature that
	// needs scope-correct visibility. A future scope-tree refactor
	// will subsume this.
	allBindings map[string]*Symbol
}

// Error is a type-checker error.
type Error struct {
	Code errs.Code
	Span token.Span
	Msg  string
}

func (e Error) Error() string                { return e.Msg }
func (e Error) GetCode() errs.Code           { return e.Code }
func (e Error) GetSpan() (start, end uint32) { return e.Span.Start, e.Span.End }

// New creates a checker with the given modules available for import
// resolution. Callers typically pass the result of BootstrapModules.
func New(modules map[string]*Scope) *Checker {
	if modules == nil {
		modules = map[string]*Scope{}
	}
	return &Checker{
		modules:     modules,
		allBindings: make(map[string]*Symbol),
	}
}

// Errors returns accumulated checker errors.
func (c *Checker) Errors() []Error { return c.errs }

// FileScope returns the top-level scope after checking. Useful for
// extracting a module's exported symbols.
func (c *Checker) FileScope() *Scope { return c.scope }

// AllBindings returns the flat fallback map of every value binding
// the checker resolved during the walk. Includes file-scope and
// nested bindings (lets, params, for-loop vars). Last-write-wins on
// name shadowing. Used by the LSP to resolve receiver references
// inside function bodies where the original scope has been popped.
func (c *Checker) AllBindings() map[string]*Symbol { return c.allBindings }

// recordBinding records a non-import symbol in the flat fallback
// map. Imports are excluded — they only make sense in their file
// scope and would pollute the fallback lookups.
func (c *Checker) recordBinding(sym *Symbol) {
	if sym == nil || sym.Kind == SymImport {
		return
	}
	if c.allBindings == nil {
		c.allBindings = make(map[string]*Symbol)
	}
	c.allBindings[sym.Name] = sym
}

// WithScope sets a pre-populated scope for Check to use instead of
// creating a fresh one. Used by multi-file module loading so all
// files in a module share one scope (Go package model).
func (c *Checker) WithScope(scope *Scope) {
	c.scope = scope
}

// Check type-checks a parsed file using ast.Walk for traversal.
func (c *Checker) Check(f *ast.File) {
	if c.scope == nil {
		c.scope = NewScope(nil, ScopeFile)
	}
	if f.Module != nil {
		c.modName = f.Module.Name.Name
	} else {
		c.modName = "main"
	}

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

// RegisterForwardDecls runs only the import + forward-declaration
// pass of Check — no body walking. Used by multi-file module loading
// to populate a shared scope before any file's bodies are checked.
// RegisterForwardDecls registers only top-level declarations (not
// imports) into the checker's scope. Used by multi-file module
// loading: all files' decls go into a shared scope first, then
// each file is Check'd individually (which handles its own imports).
func (c *Checker) RegisterForwardDecls(f *ast.File) {
	if c.scope == nil {
		c.scope = NewScope(nil, ScopeFile)
	}
	if f.Module != nil {
		c.modName = f.Module.Name.Name
	}
	for _, d := range f.Decls {
		c.registerDecl(d)
	}
}

func (c *Checker) errAt(span token.Span, code errs.Code, msg string) {
	c.errs = append(c.errs, Error{Code: code, Span: span, Msg: msg})
}

// enter is the pre-visit callback for ast.Walk.
func (c *Checker) enter(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.File:
		// Already handled imports and forward-decls above.
		return true

	case *ast.TypeDecl:
		c.checkTypeDecl(n)
		return false // children already visited by checkTypeDecl

	case *ast.AttrTypeDecl:
		c.checkAttrTypeDecl(n)
		return false

	case *ast.EnumDecl:
		return false // fully registered in forward pass

	case *ast.FuncDecl:
		c.checkFuncDecl(n)
		return false // we handle our own child walk

	case *ast.DeclDecl:
		c.checkDeclDecl(n)
		return false

	case *ast.LetDecl:
		c.checkLetDecl(n)
		return false

	case *ast.AssignStmt:
		if !c.scope.AllowsMutation() {
			c.errAt(n.SrcSpan, CodeMutationOutside, "mutation not allowed outside func bodies")
		}
		c.typeOf(n.Target)
		c.typeOf(n.Value)
		return false

	case *ast.ExprStmt:
		c.typeOf(n.Expr)
		return false

	case *ast.ReturnStmt:
		if n.Value != nil {
			vt := c.typeOf(n.Value)
			if vt != nil && c.returnType != nil && !IsAssignableTo(vt, c.returnType) {
				c.errAt(n.SrcSpan, CodeReturnTypeMismatch,
					"return type mismatch: got "+vt.String()+", want "+c.returnType.String())
			}
		}
		return false

	case *ast.LetStmt:
		c.checkLetDecl(n.Decl)
		return false

	case *ast.ForStmt:
		c.pushScope(ScopeBlock)
		iterT := c.typeOf(n.Iter)
		if n.Var != nil && iterT != nil {
			var elemT Type
			if lt, ok := iterT.(*List); ok {
				elemT = lt.Elem
			}
			if elemT != nil {
				sym := &Symbol{
					Name: n.Var.Name,
					Type: elemT,
					Kind: SymLet,
					Span: n.Var.SrcSpan,
				}
				c.scope.Define(sym)
				c.recordBinding(sym)
			}
		}
		if n.Body != nil {
			ast.Walk(n.Body, c.enter, c.leave)
		}
		c.popScope()
		return false

	case *ast.IfStmt:
		ct := c.typeOf(n.Cond)
		if ct != nil && ct != BoolType {
			c.errAt(n.Cond.Span(), CodeIfNotBool, "if condition must be bool, got "+ct.String())
		}
		c.pushScope(ScopeBlock)
		if n.Then != nil {
			ast.Walk(n.Then, c.enter, c.leave)
		}
		c.popScope()
		if n.Else != nil {
			c.pushScope(ScopeBlock)
			ast.Walk(n.Else, c.enter, c.leave)
			c.popScope()
		}
		return false
	}
	return true
}

// leave is the post-visit callback for ast.Walk. Scope management for
// ForStmt/IfStmt is handled inline in enter (they return false and
// walk their children manually).
func (c *Checker) leave(n ast.Node) {
	_ = n
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
	// Try full path first (user modules from scampi.mod). If the
	// full path is registered, accept it. Fall back to leaf lookup
	// ONLY for std modules (paths starting with "std" or single-
	// segment names like "std"). This prevents bare `import "adguard"`
	// from resolving — user modules must use their full require path.
	_, ok := c.modules[imp.Path]
	if !ok && isStdImportPath(imp.Path) {
		_, ok = c.modules[leaf]
	}
	if !ok {
		c.errAt(imp.SrcSpan, CodeUnknownModule, "unknown module: "+imp.Path)
		return
	}
	if !c.scope.Define(&Symbol{
		Name: leaf,
		Type: nil,
		Kind: SymImport,
		Span: imp.SrcSpan,
	}) {
		c.errAt(imp.SrcSpan, CodeDuplicateImport, "duplicate import: "+leaf)
	}
}

// isStdImportPath reports whether the import path looks like a std
// module path (e.g. "std", "std/posix", "std/test/matchers"). User
// module paths from scampi.mod contain dots (e.g. "scampi.dev/...").
func isStdImportPath(path string) bool {
	return !strings.Contains(path, ".")
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
	// In module main, everything is implicitly public — there are
	// no importers to hide from.
	isMain := c.modName == "main"

	switch d := d.(type) {
	case *ast.TypeDecl:
		var t Type
		if d.Fields == nil {
			t = &OpaqueType{Name: d.Name.Name}
		} else {
			t = &StructType{Name: d.Name.Name}
		}
		c.scope.Define(&Symbol{
			Name: d.Name.Name, Type: t, Kind: SymType,
			IsPublic: d.Public || isMain, Span: d.SrcSpan,
		})
	case *ast.AttrTypeDecl:
		c.scope.Define(&Symbol{
			Name: "@" + d.Name.Name,
			Type: &AttrType{Name: d.Name.Name, Doc: d.Doc},
			Kind: SymAttrType, IsPublic: true,
			Span: d.SrcSpan,
		})
	case *ast.EnumDecl:
		var variants []string
		for _, v := range d.Variants {
			variants = append(variants, v.Name)
		}
		c.scope.Define(&Symbol{
			Name: d.Name.Name,
			Type: &EnumType{Name: d.Name.Name, Variants: variants},
			Kind: SymEnum, IsPublic: d.Public || isMain, Span: d.SrcSpan,
		})
	case *ast.FuncDecl:
		c.scope.Define(&Symbol{
			Name: d.Name.Name, Kind: SymFunc,
			IsPublic: d.Public || isMain, Span: d.SrcSpan,
		})
	case *ast.DeclDecl:
		c.scope.Define(&Symbol{
			Name: d.Name.Parts[0].Name, Kind: SymDecl,
			IsPublic: d.Public || isMain, Span: d.SrcSpan,
		})
	case *ast.LetDecl:
		if !c.scope.Define(&Symbol{
			Name: d.Name.Name, Kind: SymLet,
			IsPublic: d.Public || isMain, Span: d.SrcSpan,
		}) {
			c.errAt(d.SrcSpan, CodeDuplicateLet, "duplicate let binding: "+d.Name.Name)
		}
	}
}

// Declaration checking
// -----------------------------------------------------------------------------

func (c *Checker) checkTypeDecl(d *ast.TypeDecl) {
	if d.Fields == nil {
		return // opaque type — nothing to check
	}
	sym := c.scope.Lookup(d.Name.Name)
	if sym == nil {
		return
	}
	st, ok := sym.Type.(*StructType)
	if !ok {
		return
	}
	seen := map[string]bool{}
	for _, f := range d.Fields {
		if seen[f.Name.Name] {
			c.errAt(f.Name.SrcSpan, CodeDuplicateField, "duplicate field: "+f.Name.Name)
			continue
		}
		seen[f.Name.Name] = true
		ft := c.resolveType(f.Type)
		if ft == nil {
			c.errAt(f.SrcSpan, CodeUnknownFieldType, "unknown type in field "+f.Name.Name)
			continue
		}
		c.checkFieldAttributes(f)
		st.Fields = append(st.Fields, &FieldDef{
			Name:   f.Name.Name,
			Type:   ft,
			HasDef: f.Default != nil,
		})
	}
}

// checkAttrTypeDecl resolves the field types of an attribute type
// declaration `type @name { ... }`. The attribute type's Fields slice
// is populated on the AttrType already registered during the forward
// declaration pass.
func (c *Checker) checkAttrTypeDecl(d *ast.AttrTypeDecl) {
	sym := c.scope.Lookup("@" + d.Name.Name)
	if sym == nil {
		return
	}
	at, ok := sym.Type.(*AttrType)
	if !ok {
		return
	}
	seen := map[string]bool{}
	for _, f := range d.Fields {
		if seen[f.Name.Name] {
			c.errAt(f.Name.SrcSpan, CodeDuplicateField, "duplicate field: "+f.Name.Name)
			continue
		}
		seen[f.Name.Name] = true
		ft := c.resolveType(f.Type)
		if ft == nil {
			c.errAt(f.SrcSpan, CodeUnknownAttrField, "unknown type in attribute field "+f.Name.Name)
			continue
		}
		// Attribute fields cannot themselves carry attributes —
		// keeping the model finite. Diagnose if anyone tries it.
		if len(f.Attributes) > 0 {
			c.errAt(f.Attributes[0].SrcSpan, CodeAttrFieldCarries,
				"attribute fields cannot themselves carry attributes")
		}
		at.Fields = append(at.Fields, &FieldDef{
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
	// A function with a body must declare a return type. Funcs are
	// pure (no side effects), so a body without a return is a no-op.
	// Stubs (no body) are allowed without a return type so builtin
	// declarations and forward decls keep working.
	if d.Body != nil && d.Ret == nil {
		c.errAt(d.SrcSpan, CodeError,
			"func "+d.Name.Name+" with a body requires a return type")
	}
	// Every path through a function body must end with a return
	// statement. Without this, some paths silently produce None at
	// runtime where the type system promised X.
	if d.Body != nil && d.Ret != nil && !definitelyReturns(d.Body) {
		c.errAt(d.SrcSpan, CodeNotAllPathsReturn,
			"func "+d.Name.Name+": not all paths return a value")
	}
	for _, p := range d.Params {
		c.checkFieldAttributes(p)
	}
	sym := c.scope.Lookup(d.Name.Name)
	if sym != nil {
		var params []*FieldDef
		for _, p := range d.Params {
			pt := c.resolveType(p.Type)
			params = append(params, &FieldDef{
				Name:       p.Name.Name,
				Type:       pt,
				HasDef:     p.Default != nil,
				Attributes: c.resolveFieldAttributes(p),
			})
		}
		sym.Type = &FuncType{Params: params, Ret: ret}
	}
	if d.Body != nil {
		c.pushScope(ScopeFunc)
		prevRet := c.returnType
		c.returnType = ret
		for _, p := range d.Params {
			pt := c.resolveType(p.Type)
			pSym := &Symbol{
				Name: p.Name.Name, Type: pt, Kind: SymParam, Span: p.SrcSpan,
			}
			c.scope.Define(pSym)
			c.recordBinding(pSym)
		}
		ast.Walk(d.Body, c.enter, c.leave)
		c.returnType = prevRet
		c.popScope()
	}
}

// definitelyReturns reports whether every execution path through b
// ends with a `return expr` statement. The analysis is conservative:
//
//   - A block definitely returns if its last statement is `return expr`.
//   - An if/else definitely returns if both branches definitely return.
//   - Everything else (for loops, bare statements) does not count —
//     require an explicit return after the loop.
func definitelyReturns(b *ast.Block) bool {
	if b == nil || len(b.Stmts) == 0 {
		return false
	}
	last := b.Stmts[len(b.Stmts)-1]
	switch s := last.(type) {
	case *ast.ReturnStmt:
		return s.Value != nil
	case *ast.IfStmt:
		if s.Else == nil {
			return false
		}
		return definitelyReturns(s.Then) && definitelyReturns(s.Else)
	}
	return false
}

func (c *Checker) checkDeclDecl(d *ast.DeclDecl) {
	var ret Type
	if d.Ret != nil {
		ret = c.resolveType(d.Ret)
	} else {
		c.errAt(d.SrcSpan, CodeDeclMissingReturn, "decl declaration requires a return type")
	}
	for _, p := range d.Params {
		c.checkFieldAttributes(p)
	}
	name := d.Name.Parts[0].Name
	sym := c.scope.Lookup(name)
	if sym != nil {
		var params []*FieldDef
		for _, p := range d.Params {
			pt := c.resolveType(p.Type)
			params = append(params, &FieldDef{
				Name:       p.Name.Name,
				Type:       pt,
				HasDef:     p.Default != nil,
				Attributes: c.resolveFieldAttributes(p),
			})
		}
		sym.Type = &DeclType{
			Name: name, Params: params, Ret: ret, HasBody: d.Body != nil,
		}
	}
	if d.Body != nil {
		c.pushScope(ScopeDecl)
		prevSelf := c.selfFields
		var stepParams []*FieldDef
		for _, p := range d.Params {
			pt := c.resolveType(p.Type)
			fd := &FieldDef{Name: p.Name.Name, Type: pt, HasDef: p.Default != nil}
			stepParams = append(stepParams, fd)
			pSym := &Symbol{
				Name: p.Name.Name, Type: pt, Kind: SymParam, Span: p.SrcSpan,
			}
			c.scope.Define(pSym)
			c.recordBinding(pSym)
		}
		c.selfFields = stepParams
		ast.Walk(d.Body, c.enter, c.leave)
		c.selfFields = prevSelf
		c.popScope()
	}
}

func (c *Checker) checkLetDecl(d *ast.LetDecl) {
	var declared Type
	if d.Type != nil {
		declared = c.resolveType(d.Type)
	}
	inferred := c.typeOf(d.Value)
	if declared != nil && inferred != nil {
		if !IsAssignableTo(inferred, declared) {
			c.errAt(d.SrcSpan, CodeLetTypeMismatch,
				"let type mismatch: got "+inferred.String()+", want "+declared.String())
		}
	}
	resolved := inferred
	if declared != nil {
		resolved = declared
	}
	// Update a pre-registered symbol (top-level forward-decl) or
	// define a new one (nested let inside a body).
	sym := c.scope.Lookup(d.Name.Name)
	if sym != nil && sym.Kind == SymLet {
		sym.Type = resolved
	} else {
		sym = &Symbol{
			Name: d.Name.Name,
			Type: resolved,
			Kind: SymLet,
			Span: d.SrcSpan,
		}
		c.scope.Define(sym)
	}
	c.recordBinding(sym)
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
		if sym != nil && (sym.Kind == SymType || sym.Kind == SymEnum || sym.Kind == SymDecl) {
			return sym.Type
		}
		c.errAt(t.SrcSpan, CodeUnknownType, "unknown type: "+name)
		return nil
	}
	// Dotted name: resolve first part as module import, walk into it.
	first := t.Name.Parts[0].Name
	sym := c.scope.Lookup(first)
	if sym == nil || sym.Kind != SymImport {
		c.errAt(t.SrcSpan, CodeUnknownType, "unknown type: "+first)
		return nil
	}
	return c.resolveModuleMember(first, t.Name.Parts[1:], t.SrcSpan)
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
	}
	return nil
}

func (c *Checker) resolveGenericType(t *ast.GenericType) Type {
	switch t.Name.Name {
	case "list":
		if len(t.Args) != 1 {
			c.errAt(t.SrcSpan, CodeGenericArgCount, "list takes exactly 1 type argument")
			return nil
		}
		elem := c.resolveType(t.Args[0])
		if elem == nil {
			return nil
		}
		return &List{Elem: elem}
	case "map":
		if len(t.Args) != 2 {
			c.errAt(t.SrcSpan, CodeGenericArgCount, "map takes exactly 2 type arguments")
			return nil
		}
		k := c.resolveType(t.Args[0])
		v := c.resolveType(t.Args[1])
		if k == nil || v == nil {
			return nil
		}
		return &Map{Key: k, Value: v}
	case "block":
		if len(t.Args) != 1 {
			c.errAt(t.SrcSpan, CodeGenericArgCount, "block takes exactly 1 type argument")
			return nil
		}
		inner := c.resolveType(t.Args[0])
		if inner == nil {
			return nil
		}
		return &BlockType{Inner: inner}
	}
	c.errAt(t.SrcSpan, CodeUnknownGenericType, "unknown generic type: "+t.Name.Name)
	return nil
}
