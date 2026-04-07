// SPDX-License-Identifier: GPL-3.0-only

package parse

import (
	"testing"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/lex"
)

// parseFile is a test helper that parses src and fails the test on
// any lexer or parser error.
func parseFile(t *testing.T, src string) *ast.File {
	t.Helper()
	l := lex.New("test.scampi", []byte(src))
	p := New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		t.Fatalf("lexer errors: %v", errs)
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parser errors: %v", errs)
	}
	return f
}

// parseExprOnly parses src as a single expression (wraps it in a
// let-binding to make the grammar happy, then extracts the value).
func parseExprOnly(t *testing.T, src string) ast.Expr {
	t.Helper()
	f := parseFile(t, "module main\nlet x = "+src)
	if len(f.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(f.Decls))
	}
	let, ok := f.Decls[0].(*ast.LetDecl)
	if !ok {
		t.Fatalf("expected LetDecl, got %T", f.Decls[0])
	}
	return let.Value
}

// imports
// -----------------------------------------------------------------------------

func TestParseImport(t *testing.T) {
	f := parseFile(t, `
module main
import "std"
`)
	if len(f.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(f.Imports))
	}
	if f.Imports[0].Path != "std" {
		t.Errorf("import path: got %q, want %q", f.Imports[0].Path, "std")
	}
}

func TestParseMultipleImports(t *testing.T) {
	f := parseFile(t, `
module main
import "std"
import "std/rest"
import "codeberg.org/scampi-dev/modules/unifi"
`)
	if len(f.Imports) != 3 {
		t.Fatalf("expected 3 imports, got %d", len(f.Imports))
	}
	wants := []string{"std", "std/rest", "codeberg.org/scampi-dev/modules/unifi"}
	for i, want := range wants {
		if f.Imports[i].Path != want {
			t.Errorf("import %d: got %q, want %q", i, f.Imports[i].Path, want)
		}
	}
}

// struct decl
// -----------------------------------------------------------------------------

func TestParseStruct(t *testing.T) {
	f := parseFile(t, `
module main
struct User {
    name: string
    groups: list[string] = []
    shell: string = "/bin/bash"
}
`)
	if len(f.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(f.Decls))
	}
	s, ok := f.Decls[0].(*ast.StructDecl)
	if !ok {
		t.Fatalf("expected StructDecl, got %T", f.Decls[0])
	}
	if s.Name.Name != "User" {
		t.Errorf("struct name: got %q, want %q", s.Name.Name, "User")
	}
	if len(s.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(s.Fields))
	}
	if s.Fields[0].Name.Name != "name" {
		t.Errorf("field 0 name: got %q, want %q", s.Fields[0].Name.Name, "name")
	}
	if s.Fields[1].Default == nil {
		t.Error("field 1 should have default")
	}
}

// enum decl
// -----------------------------------------------------------------------------

func TestParseEnum(t *testing.T) {
	f := parseFile(t, `
module main
enum PkgState { present, absent, latest }
`)
	e := f.Decls[0].(*ast.EnumDecl)
	if e.Name.Name != "PkgState" {
		t.Errorf("enum name: got %q", e.Name.Name)
	}
	if len(e.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(e.Variants))
	}
	wants := []string{"present", "absent", "latest"}
	for i, w := range wants {
		if e.Variants[i].Name != w {
			t.Errorf("variant %d: got %q, want %q", i, e.Variants[i].Name, w)
		}
	}
}

// func decl
// -----------------------------------------------------------------------------

func TestParseFunc(t *testing.T) {
	f := parseFile(t, `
module main
func build_url(host: string, path: string = "/") string {
    return "done"
}
`)
	fn := f.Decls[0].(*ast.FuncDecl)
	if fn.Name.Name != "build_url" {
		t.Errorf("func name: got %q", fn.Name.Name)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(fn.Params))
	}
	if fn.Params[0].Name.Name != "host" {
		t.Errorf("param 0 name: %q", fn.Params[0].Name.Name)
	}
	if fn.Params[1].Default == nil {
		t.Error("param 1 should have default")
	}
	if fn.Ret == nil {
		t.Error("func should have return type")
	}
	if fn.Body == nil || len(fn.Body.Stmts) == 0 {
		t.Fatal("func body empty")
	}
	if _, ok := fn.Body.Stmts[0].(*ast.ReturnStmt); !ok {
		t.Errorf("expected ReturnStmt, got %T", fn.Body.Stmts[0])
	}
}

// decl decl
// -----------------------------------------------------------------------------

func TestParseDeclWithBody(t *testing.T) {
	f := parseFile(t, `
module main
decl create_user(name: string, shell: string = "/bin/bash") Step {
    std.user { name = self.name, shell = self.shell }
}
`)
	s := f.Decls[0].(*ast.DeclDecl)
	if len(s.Name.Parts) != 1 || s.Name.Parts[0].Name != "create_user" {
		t.Errorf("decl name wrong")
	}
	if len(s.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(s.Params))
	}
	if s.Body == nil || len(s.Body.Stmts) != 1 {
		t.Fatalf("expected 1 body stmt, got %v", s.Body)
	}
}

func TestParseDeclStub(t *testing.T) {
	f := parseFile(t, `
module main
decl pkg(packages: list[string], state: PkgState = PkgState.present) Step
`)
	s := f.Decls[0].(*ast.DeclDecl)
	if s.Body != nil {
		t.Error("stub should have no body")
	}
	if s.Ret == nil {
		t.Error("stub should have return type")
	}
}

func TestParseDeclDottedName(t *testing.T) {
	f := parseFile(t, `
module main
decl container.instance(name: string) Step
`)
	s := f.Decls[0].(*ast.DeclDecl)
	if len(s.Name.Parts) != 2 {
		t.Fatalf("expected dotted name with 2 parts, got %d", len(s.Name.Parts))
	}
	if s.Name.Parts[0].Name != "container" || s.Name.Parts[1].Name != "instance" {
		t.Errorf("dotted name parts wrong: %v", s.Name.Parts)
	}
}

// let
// -----------------------------------------------------------------------------

func TestParseLet(t *testing.T) {
	f := parseFile(t, `
module main
let version = "1.2.3"
`)
	d := f.Decls[0].(*ast.LetDecl)
	if d.Name.Name != "version" {
		t.Errorf("let name: %q", d.Name.Name)
	}
	if _, ok := d.Value.(*ast.StringLit); !ok {
		t.Errorf("expected StringLit value, got %T", d.Value)
	}
}

func TestParseLetWithType(t *testing.T) {
	f := parseFile(t, `
module main
let n: int = 42
`)
	d := f.Decls[0].(*ast.LetDecl)
	if d.Type == nil {
		t.Error("let should have type annotation")
	}
}

// expressions
// -----------------------------------------------------------------------------

func TestParseArithmetic(t *testing.T) {
	e := parseExprOnly(t, "1 + 2 * 3")
	bin, ok := e.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr at top, got %T", e)
	}
	// Precedence: 1 + (2 * 3)
	if _, ok := bin.Right.(*ast.BinaryExpr); !ok {
		t.Errorf("expected nested BinaryExpr on right, got %T", bin.Right)
	}
}

func TestParseComparison(t *testing.T) {
	e := parseExprOnly(t, "a < b && c == d")
	bin := e.(*ast.BinaryExpr)
	// `&&` is lower precedence than `<` and `==`, so top is &&
	// with left = (a<b), right = (c==d)
	if _, ok := bin.Left.(*ast.BinaryExpr); !ok {
		t.Errorf("expected && left to be BinaryExpr, got %T", bin.Left)
	}
}

func TestParseMemberAccess(t *testing.T) {
	e := parseExprOnly(t, "std.pkg.present")
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		t.Fatalf("expected SelectorExpr, got %T", e)
	}
	if sel.Sel.Name != "present" {
		t.Errorf("outer selector: %q", sel.Sel.Name)
	}
}

func TestParseCall(t *testing.T) {
	e := parseExprOnly(t, `std.env("HOME", "/root")`)
	call, ok := e.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", e)
	}
	if len(call.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(call.Args))
	}
}

func TestParseIndex(t *testing.T) {
	e := parseExprOnly(t, `xs[0]`)
	idx, ok := e.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("expected IndexExpr, got %T", e)
	}
	_ = idx
}

func TestParseList(t *testing.T) {
	e := parseExprOnly(t, `[1, 2, 3]`)
	l, ok := e.(*ast.ListLit)
	if !ok {
		t.Fatalf("expected ListLit, got %T", e)
	}
	if len(l.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(l.Items))
	}
}

func TestParseEmptyList(t *testing.T) {
	e := parseExprOnly(t, `[]`)
	l := e.(*ast.ListLit)
	if len(l.Items) != 0 {
		t.Errorf("expected empty list, got %d items", len(l.Items))
	}
}

func TestParseMap(t *testing.T) {
	e := parseExprOnly(t, `{"a": 1, "b": 2}`)
	m, ok := e.(*ast.MapLit)
	if !ok {
		t.Fatalf("expected MapLit, got %T", e)
	}
	if len(m.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m.Entries))
	}
}

func TestParseStructLit(t *testing.T) {
	e := parseExprOnly(t, `User { name = "alice", age = 30 }`)
	s, ok := e.(*ast.StructLit)
	if !ok {
		t.Fatalf("expected StructLit, got %T", e)
	}
	if len(s.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(s.Fields))
	}
	if s.Type == nil {
		t.Error("struct lit should have type")
	}
}

func TestParseInferredStructLit(t *testing.T) {
	e := parseExprOnly(t, `{ name = "alice" }`)
	s, ok := e.(*ast.StructLit)
	if !ok {
		t.Fatalf("expected StructLit, got %T", e)
	}
	if s.Type != nil {
		t.Error("inferred struct lit should have no type")
	}
}

func TestParseIfExpr(t *testing.T) {
	e := parseExprOnly(t, `if x { 1 } else { 2 }`)
	ife, ok := e.(*ast.IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr, got %T", e)
	}
	_ = ife
}

func TestParseListComp(t *testing.T) {
	e := parseExprOnly(t, `[x * 2 for x in xs if x > 0]`)
	c, ok := e.(*ast.ListComp)
	if !ok {
		t.Fatalf("expected ListComp, got %T", e)
	}
	if c.Cond == nil {
		t.Error("list comp should have condition")
	}
}

// statements
// -----------------------------------------------------------------------------

func TestParseForStmt(t *testing.T) {
	f := parseFile(t, `
module main
func f() list[int] {
    for x in xs {
        x + 1
    }
    return []
}
`)
	fn := f.Decls[0].(*ast.FuncDecl)
	if _, ok := fn.Body.Stmts[0].(*ast.ForStmt); !ok {
		t.Errorf("expected ForStmt, got %T", fn.Body.Stmts[0])
	}
}

func TestParseIfStmt(t *testing.T) {
	f := parseFile(t, `
module main
func f() int {
    if x > 0 {
        return 1
    } else {
        return 2
    }
}
`)
	fn := f.Decls[0].(*ast.FuncDecl)
	ifs, ok := fn.Body.Stmts[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("expected IfStmt, got %T", fn.Body.Stmts[0])
	}
	if ifs.Else == nil {
		t.Error("if stmt should have else")
	}
}

// optional types
// -----------------------------------------------------------------------------

func TestParseOptionalType(t *testing.T) {
	f := parseFile(t, `
module main
struct X { name: string? }
`)
	s := f.Decls[0].(*ast.StructDecl)
	if _, ok := s.Fields[0].Type.(*ast.OptionalType); !ok {
		t.Errorf("expected OptionalType, got %T", s.Fields[0].Type)
	}
}

func TestParseGenericType(t *testing.T) {
	f := parseFile(t, `
module main
struct X { xs: list[string] }
`)
	s := f.Decls[0].(*ast.StructDecl)
	if _, ok := s.Fields[0].Type.(*ast.GenericType); !ok {
		t.Errorf("expected GenericType, got %T", s.Fields[0].Type)
	}
}

func TestParseMapType(t *testing.T) {
	f := parseFile(t, `
module main
struct X { m: map[string, int] }
`)
	s := f.Decls[0].(*ast.StructDecl)
	gt, ok := s.Fields[0].Type.(*ast.GenericType)
	if !ok {
		t.Fatalf("expected GenericType, got %T", s.Fields[0].Type)
	}
	if len(gt.Args) != 2 {
		t.Errorf("expected 2 generic args, got %d", len(gt.Args))
	}
}

// real-world snippet
// -----------------------------------------------------------------------------

func TestParseRealSnippet(t *testing.T) {
	src := `
module main
import "std"

let vps_host = std.secret("vps.host")

struct User {
    name: string
    groups: list[string] = []
}

decl create_user(u: User) Step {
    std.user {
        name = u.name
        groups = u.groups
    }
}
`
	f := parseFile(t, src)
	if len(f.Imports) != 1 {
		t.Errorf("expected 1 import, got %d", len(f.Imports))
	}
	if len(f.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(f.Decls))
	}
}
