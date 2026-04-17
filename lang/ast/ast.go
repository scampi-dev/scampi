// SPDX-License-Identifier: GPL-3.0-only

// Package ast defines the abstract syntax tree for scampi.
// Every node carries a Span so diagnostics can point back to source.
package ast

import (
	"scampi.dev/scampi/lang/token"
)

// Node is the root interface for every AST node.
type Node interface {
	Span() token.Span
	astNode() // sealed: only types in this package implement Node
}

// File is the root of a parsed source file. It contains the import
// declarations followed by top-level declarations and statements in
// the order they appear in source.
type File struct {
	Name    string // filename (for diagnostics)
	Module  *ModuleDecl
	Imports []*ImportDecl
	Decls   []Decl // top-level declarations
	Stmts   []Stmt // top-level statements (step invocations)
	SrcSpan token.Span
}

func (f *File) Span() token.Span { return f.SrcSpan }
func (*File) astNode()           {}

// -----------------------------------------------------------------------------
// Declarations
// -----------------------------------------------------------------------------

// Decl is any top-level or block-level declaration.
type Decl interface {
	Node
	declNode()
}

// ModuleDecl is a `module <name>` declaration at the top of a file.
// All files in a directory must agree on the module name.
// `module main` marks an entry-point file for `scampi apply`.
type ModuleDecl struct {
	Name    *Ident
	SrcSpan token.Span
}

func (d *ModuleDecl) Span() token.Span { return d.SrcSpan }
func (*ModuleDecl) astNode()           {}

// ImportDecl is an `import "path"` statement.
type ImportDecl struct {
	Path    string // unquoted import path
	SrcSpan token.Span
}

func (d *ImportDecl) Span() token.Span { return d.SrcSpan }
func (*ImportDecl) astNode()           {}
func (*ImportDecl) declNode()          {}

// TypeDecl is a type declaration. With fields: `type Name { field: type, ... }`.
// Without fields (nil): `type Name` — an opaque forward declaration.
type TypeDecl struct {
	Name    *Ident
	Fields  []*Field // nil for opaque types
	Public  bool
	SrcSpan token.Span
}

func (d *TypeDecl) Span() token.Span { return d.SrcSpan }
func (*TypeDecl) astNode()           {}
func (*TypeDecl) declNode()          {}

// AttrTypeDecl is an attribute type declaration: `type @name { ... }`.
// Attribute types live in a separate `@`-prefixed namespace from
// regular types — they cannot be used in type expressions or
// instantiated as struct literals. They're consumed only as `@name`
// or `@name(args)` decorations on Fields and other annotatable
// positions.
//
// Marker attribute types have no fields: `type @marker {}`.
// Otherwise the field schema describes the accepted arguments.
//
// Doc carries the contiguous `//` line comments immediately
// preceding the declaration in the source. The comment block is
// joined with newlines and stripped of its `// ` prefix. Linker
// behaviours read it to render rich Help text on validation
// diagnostics, and the LSP renders it as hover documentation.
type AttrTypeDecl struct {
	Name    *Ident
	Fields  []*Field // empty list for markers; never nil
	Doc     string
	SrcSpan token.Span
}

func (d *AttrTypeDecl) Span() token.Span { return d.SrcSpan }
func (*AttrTypeDecl) astNode()           {}
func (*AttrTypeDecl) declNode()          {}

// EnumDecl is an `enum Name { variant, ... }` declaration.
type EnumDecl struct {
	Name     *Ident
	Variants []*Ident
	Public   bool
	SrcSpan  token.Span
}

func (d *EnumDecl) Span() token.Span { return d.SrcSpan }
func (*EnumDecl) astNode()           {}
func (*EnumDecl) declNode()          {}

// FuncDecl is a `func Name(params) ReturnType { body }` declaration.
type FuncDecl struct {
	Name    *Ident
	Params  []*Field
	Ret     TypeExpr // nil for void (not allowed in v0)
	Body    *Block
	Public  bool
	SrcSpan token.Span
}

func (d *FuncDecl) Span() token.Span { return d.SrcSpan }
func (*FuncDecl) astNode()           {}
func (*FuncDecl) declNode()          {}

// DeclDecl is a `decl Name(params) OutputType { body }` declaration.
// Body may be nil for stubs. Name may be dotted (e.g. container.instance).
type DeclDecl struct {
	Name    *DottedName
	Params  []*Field
	Ret     TypeExpr // nil means defaults to Step
	Body    *Block   // nil for stubs (builtins with no implementation)
	Public  bool
	SrcSpan token.Span
}

func (d *DeclDecl) Span() token.Span { return d.SrcSpan }
func (*DeclDecl) astNode()           {}
func (*DeclDecl) declNode()          {}

// LetDecl is a `let name: type = expr` binding.
type LetDecl struct {
	Name    *Ident
	Type    TypeExpr // optional type annotation
	Value   Expr
	Public  bool
	SrcSpan token.Span
}

func (d *LetDecl) Span() token.Span { return d.SrcSpan }
func (*LetDecl) astNode()           {}
func (*LetDecl) declNode()          {}

// -----------------------------------------------------------------------------
// Statements
// -----------------------------------------------------------------------------

// Stmt is a statement: a declaration, control flow, or bare expression.
type Stmt interface {
	Node
	stmtNode()
}

// LetStmt wraps LetDecl when used as a statement in a block.
type LetStmt struct {
	Decl *LetDecl
}

func (s *LetStmt) Span() token.Span { return s.Decl.SrcSpan }
func (*LetStmt) astNode()           {}
func (*LetStmt) stmtNode()          {}

// ForStmt is a `for name in expr { body }` statement.
type ForStmt struct {
	Var     *Ident
	Iter    Expr
	Body    *Block
	SrcSpan token.Span
}

func (s *ForStmt) Span() token.Span { return s.SrcSpan }
func (*ForStmt) astNode()           {}
func (*ForStmt) stmtNode()          {}

// IfStmt is an `if cond { then } else { else_ }` statement (else optional).
type IfStmt struct {
	Cond    Expr
	Then    *Block
	Else    *Block // nil if no else
	SrcSpan token.Span
}

func (s *IfStmt) Span() token.Span { return s.SrcSpan }
func (*IfStmt) astNode()           {}
func (*IfStmt) stmtNode()          {}

// ReturnStmt is a `return expr` statement. Value may be nil.
type ReturnStmt struct {
	Value   Expr
	SrcSpan token.Span
}

func (s *ReturnStmt) Span() token.Span { return s.SrcSpan }
func (*ReturnStmt) astNode()           {}
func (*ReturnStmt) stmtNode()          {}

// ExprStmt is an expression used as a statement (e.g. a step invocation).
type ExprStmt struct {
	Expr Expr
}

func (s *ExprStmt) Span() token.Span { return s.Expr.Span() }
func (*ExprStmt) astNode()           {}
func (*ExprStmt) stmtNode()          {}

// AssignStmt is `target[i] = value` or `target.field = value`.
// Only valid inside func bodies; the type checker enforces scope.
type AssignStmt struct {
	Target  Expr // IndexExpr or SelectorExpr
	Value   Expr
	SrcSpan token.Span
}

func (s *AssignStmt) Span() token.Span { return s.SrcSpan }
func (*AssignStmt) astNode()           {}
func (*AssignStmt) stmtNode()          {}

// -----------------------------------------------------------------------------
// Blocks
// -----------------------------------------------------------------------------

// Block is a brace-delimited sequence of statements.
type Block struct {
	Stmts   []Stmt
	SrcSpan token.Span
}

func (b *Block) Span() token.Span { return b.SrcSpan }
func (*Block) astNode()           {}

// -----------------------------------------------------------------------------
// Expressions
// -----------------------------------------------------------------------------

// Expr is any expression.
type Expr interface {
	Node
	exprNode()
}

// Ident is a bare identifier.
type Ident struct {
	Name    string
	SrcSpan token.Span
}

func (e *Ident) Span() token.Span { return e.SrcSpan }
func (*Ident) astNode()           {}
func (*Ident) exprNode()          {}

// DottedName is a dotted path like `foo.bar.baz`.
type DottedName struct {
	Parts   []*Ident
	SrcSpan token.Span
}

func (e *DottedName) Span() token.Span { return e.SrcSpan }
func (*DottedName) astNode()           {}
func (*DottedName) exprNode()          {}

// ParenExpr wraps an expression in parentheses. Preserving this in
// the AST lets the formatter re-emit the parens faithfully.
type ParenExpr struct {
	Inner   Expr
	SrcSpan token.Span
}

func (e *ParenExpr) Span() token.Span { return e.SrcSpan }
func (*ParenExpr) astNode()           {}
func (*ParenExpr) exprNode()          {}

// IntLit is an integer literal.
type IntLit struct {
	Value   int64
	Raw     string // original text (for diagnostics)
	SrcSpan token.Span
}

func (e *IntLit) Span() token.Span { return e.SrcSpan }
func (*IntLit) astNode()           {}
func (*IntLit) exprNode()          {}

// StringLit is a string literal. For non-interpolated strings, Parts
// has one StringText entry. Interpolated strings alternate StringText
// and embedded Expr parts.
type StringLit struct {
	Parts   []StringPart
	SrcSpan token.Span
}

func (e *StringLit) Span() token.Span { return e.SrcSpan }
func (*StringLit) astNode()           {}
func (*StringLit) exprNode()          {}

// StringPart is either literal text or an embedded expression.
type StringPart interface {
	stringPart()
}

// StringText is a literal text chunk of a string. Raw is the source
// text (escapes not yet resolved); AST consumers call Resolve() to get
// the processed value.
type StringText struct {
	Raw     string
	SrcSpan token.Span
}

func (*StringText) stringPart() {}

// StringInterp is an expression embedded in a string via ${expr}.
type StringInterp struct {
	Expr    Expr
	SrcSpan token.Span
}

func (*StringInterp) stringPart() {}

// BoolLit is a `true` or `false` literal.
type BoolLit struct {
	Value   bool
	SrcSpan token.Span
}

func (e *BoolLit) Span() token.Span { return e.SrcSpan }
func (*BoolLit) astNode()           {}
func (*BoolLit) exprNode()          {}

type NoneLit struct {
	SrcSpan token.Span
}

func (e *NoneLit) Span() token.Span { return e.SrcSpan }
func (*NoneLit) astNode()           {}
func (*NoneLit) exprNode()          {}

type SelfLit struct {
	SrcSpan token.Span
}

func (e *SelfLit) Span() token.Span { return e.SrcSpan }
func (*SelfLit) astNode()           {}
func (*SelfLit) exprNode()          {}

// ListLit is a list literal `[a, b, c]`.
type ListLit struct {
	Items   []Expr
	SrcSpan token.Span
}

func (e *ListLit) Span() token.Span { return e.SrcSpan }
func (*ListLit) astNode()           {}
func (*ListLit) exprNode()          {}

// MapLit is a map literal `{"key": value, ...}`.
type MapLit struct {
	Entries []*MapEntry
	SrcSpan token.Span
}

func (e *MapLit) Span() token.Span { return e.SrcSpan }
func (*MapLit) astNode()           {}
func (*MapLit) exprNode()          {}

// MapEntry is a single key:value pair in a map literal.
type MapEntry struct {
	Key   Expr
	Value Expr
}

// StructLit is a struct literal `TypeName { field = value, ... }`
// or an inferred-type bare block `{ field = value }`. For step/deploy
// invocations that have bodies, Body holds statements that appear
// after (or interleaved with) the field inits.
type StructLit struct {
	Type    TypeExpr // nil for context-inferred literals
	Fields  []*FieldInit
	Body    []Stmt // statements in the block (step invocations, let, for, if)
	SrcSpan token.Span
}

func (e *StructLit) Span() token.Span { return e.SrcSpan }
func (*StructLit) astNode()           {}
func (*StructLit) exprNode()          {}

// FieldInit is a single `field = value` binding in a struct/step literal.
type FieldInit struct {
	Name    *Ident
	Value   Expr
	SrcSpan token.Span
}

// BlockExpr is `expr { stmts }` — fills a block[T] value with a
// statement body to produce a T. The target expression must evaluate
// to a block[T] type.
type BlockExpr struct {
	Target  Expr
	Body    *Block
	SrcSpan token.Span
}

func (e *BlockExpr) Span() token.Span { return e.SrcSpan }
func (*BlockExpr) astNode()           {}
func (*BlockExpr) exprNode()          {}

// CallExpr is a function call `fn(arg, ...)` with positional or
// keyword arguments.
//
// UFCS is true when the type checker has identified this call as a
// uniform-function-call-syntax site: `x.f(args)` semantically calls
// `f(x, args...)`. The receiver `x` lives in `Fn.(*SelectorExpr).X`
// and the callee name in `Fn.(*SelectorExpr).Sel`. The AST shape is
// preserved (no rewrite) so source-level tooling and diagnostics can
// still point at the original syntax; downstream consumers (eval,
// codegen) check this flag and dispatch accordingly.
//
// UFCSModule is non-empty when the resolved UFCS function lives in
// an imported module rather than the file's top-level scope. The
// evaluator uses it to look up the function via the module's MapVal
// instead of a bare env lookup. Empty for local UFCS (function
// declared in the current file).
type CallExpr struct {
	Fn         Expr
	Args       []*CallArg
	UFCS       bool
	UFCSModule string
	SrcSpan    token.Span
}

// CallArg is a positional or keyword argument. Name is nil for positional.
type CallArg struct {
	Name  *Ident
	Value Expr
}

func (e *CallExpr) Span() token.Span { return e.SrcSpan }
func (*CallExpr) astNode()           {}
func (*CallExpr) exprNode()          {}

// SelectorExpr is a field access `x.field`.
type SelectorExpr struct {
	X       Expr
	Sel     *Ident
	SrcSpan token.Span
}

func (e *SelectorExpr) Span() token.Span { return e.SrcSpan }
func (*SelectorExpr) astNode()           {}
func (*SelectorExpr) exprNode()          {}

// IndexExpr is an index expression `x[i]`.
type IndexExpr struct {
	X       Expr
	Index   Expr
	SrcSpan token.Span
}

func (e *IndexExpr) Span() token.Span { return e.SrcSpan }
func (*IndexExpr) astNode()           {}
func (*IndexExpr) exprNode()          {}

// BinaryExpr is a binary expression `left op right`.
type BinaryExpr struct {
	Op      token.Kind
	Left    Expr
	Right   Expr
	SrcSpan token.Span
}

func (e *BinaryExpr) Span() token.Span { return e.SrcSpan }
func (*BinaryExpr) astNode()           {}
func (*BinaryExpr) exprNode()          {}

// UnaryExpr is a unary expression `op x` (e.g. `!cond`, `-x`).
type UnaryExpr struct {
	Op      token.Kind
	X       Expr
	SrcSpan token.Span
}

func (e *UnaryExpr) Span() token.Span { return e.SrcSpan }
func (*UnaryExpr) astNode()           {}
func (*UnaryExpr) exprNode()          {}

// IfExpr is an `if`-expression with mandatory else branch.
type IfExpr struct {
	Cond    Expr
	Then    Expr
	Else    Expr
	SrcSpan token.Span
}

func (e *IfExpr) Span() token.Span { return e.SrcSpan }
func (*IfExpr) astNode()           {}
func (*IfExpr) exprNode()          {}

// ListComp is a list comprehension `[expr for var in iter if cond]`.
type ListComp struct {
	Expr    Expr
	Var     *Ident
	Iter    Expr
	Cond    Expr // optional, nil if no filter
	SrcSpan token.Span
}

func (e *ListComp) Span() token.Span { return e.SrcSpan }
func (*ListComp) astNode()           {}
func (*ListComp) exprNode()          {}

// MapComp is a map comprehension `{k: v for var in iter if cond}`.
type MapComp struct {
	Key     Expr
	Value   Expr
	Vars    []*Ident // 1 or 2 variables
	Iter    Expr
	Cond    Expr
	SrcSpan token.Span
}

func (e *MapComp) Span() token.Span { return e.SrcSpan }
func (*MapComp) astNode()           {}
func (*MapComp) exprNode()          {}

// -----------------------------------------------------------------------------
// Fields and Type expressions
// -----------------------------------------------------------------------------

// Field is a typed field declaration: name: type = default
// Used in struct decls, step/func params. Attributes are prefix
// annotations like `@nonempty` or `@path(absolute=true)` that decorate
// the field. The lang itself attaches no semantics — the linker and
// LSP consume them.
type Field struct {
	Name       *Ident
	Type       TypeExpr
	Default    Expr // optional
	Attributes []*Attribute
	SrcSpan    token.Span
}

// Attribute is a prefix annotation `@name` or `@name(args)` attached
// to a Field, declaration, or other annotatable position. The lang
// parses and validates the structural shape but does not interpret
// the semantics — that lives in the linker (Go behaviour) and LSP
// (UX providers).
type Attribute struct {
	Name        *DottedName // @name or @module.name
	Positionals []Expr      // bare positional args
	Named       []*AttrArg  // keyword args
	SrcSpan     token.Span
}

func (a *Attribute) Span() token.Span { return a.SrcSpan }
func (*Attribute) astNode()           {}

// AttrArg is a single keyword argument inside an attribute call:
// `name=value`. Positional arguments are stored directly as Expr on
// the Attribute, not wrapped in AttrArg.
type AttrArg struct {
	Name    *Ident
	Value   Expr
	SrcSpan token.Span
}

func (a *AttrArg) Span() token.Span { return a.SrcSpan }
func (*AttrArg) astNode()           {}

// TypeExpr is a type expression appearing in declarations.
type TypeExpr interface {
	Node
	typeNode()
}

// NamedType is a simple or dotted type name (string, int, User, std.pkg).
type NamedType struct {
	Name    *DottedName
	SrcSpan token.Span
}

func (t *NamedType) Span() token.Span { return t.SrcSpan }
func (*NamedType) astNode()           {}
func (*NamedType) typeNode()          {}

// GenericType is a parameterized type like `list[T]` or `map[K, V]`.
type GenericType struct {
	Name    *Ident
	Args    []TypeExpr
	SrcSpan token.Span
}

func (t *GenericType) Span() token.Span { return t.SrcSpan }
func (*GenericType) astNode()           {}
func (*GenericType) typeNode()          {}

// OptionalType is a nullable type `T?`.
type OptionalType struct {
	Inner   TypeExpr
	SrcSpan token.Span
}

func (t *OptionalType) Span() token.Span { return t.SrcSpan }
func (*OptionalType) astNode()           {}
func (*OptionalType) typeNode()          {}
