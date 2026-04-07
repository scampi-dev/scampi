// SPDX-License-Identifier: GPL-3.0-only

package check

import (
	"testing"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/std"
)

func parseAndCheck(t *testing.T, src string) ([]Error, *ast.File) {
	t.Helper()
	l := lex.New("test.scampi", []byte(src))
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		t.Fatalf("lexer errors: %v", errs)
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parser errors: %v", errs)
	}
	modules, err := BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	c := New(modules)
	c.Check(f)
	return c.Errors(), f
}

func expectNoErrors(t *testing.T, src string) {
	t.Helper()
	errs, _ := parseAndCheck(t, src)
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func expectError(t *testing.T, src string, substr string) {
	t.Helper()
	errs, _ := parseAndCheck(t, src)
	if len(errs) == 0 {
		t.Fatal("expected an error, got none")
	}
	for _, e := range errs {
		if contains(e.Msg, substr) {
			return
		}
	}
	t.Errorf("expected error containing %q, got: %v", substr, errs)
}

func contains(s, sub string) bool {
	return len(sub) <= len(s) && searchStr(s, sub)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Imports
// -----------------------------------------------------------------------------

func TestCheckImportStd(t *testing.T) {
	expectNoErrors(t, `
module main
import "std"
`)
}

func TestCheckImportUnknown(t *testing.T) {
	expectError(t, `
module main
import "nonexistent"
`, "unknown module")
}

// Type declarations
// -----------------------------------------------------------------------------

func TestCheckTypeWithPrimitiveFields(t *testing.T) {
	expectNoErrors(t, `
module main
type User {
    name: string
    age: int
    admin: bool
}
`)
}

func TestCheckTypeWithOptionalField(t *testing.T) {
	expectNoErrors(t, `
module main
type Config {
    host: string
    port: int?
}
`)
}

func TestCheckTypeWithGenericField(t *testing.T) {
	expectNoErrors(t, `
module main
type Team {
    members: list[string]
    meta: map[string, any]
}
`)
}

func TestCheckTypeWithUnknownFieldType(t *testing.T) {
	expectError(t, `
module main
type Bad {
    x: NonExistentType
}
`, "unknown type")
}

// Enum declarations
// -----------------------------------------------------------------------------

func TestCheckOpaqueType(t *testing.T) {
	expectNoErrors(t, `
module main
type Step
decl copy(src: string) Step
`)
}

func TestCheckOpaqueTypeCannotConstruct(t *testing.T) {
	expectError(t, `
module main
type Opaque
let s = Opaque {}
`, "cannot construct opaque type")
}

func TestCheckOpaqueTypeUsedInSignature(t *testing.T) {
	expectNoErrors(t, `
module main
type Step
func takes_step(s: Step) int {
    return 42
}
`)
}

func TestCheckBlockExpr(t *testing.T) {
	expectNoErrors(t, `
module main
import "std"
import "std/posix"
let vps = posix.local { name = "dev" }
let d = std.deploy(name = "web", targets = [vps])
d { posix.dir { path = "/tmp/test" } }
`)
}

func TestCheckBlockExprInline(t *testing.T) {
	expectNoErrors(t, `
module main
import "std"
import "std/posix"

std.deploy(name = "web", targets = [posix.local { name = "dev" }]) {
    posix.dir { path = "/tmp/test" }
}
`)
}

// Enum declarations
// -----------------------------------------------------------------------------

func TestCheckEnum(t *testing.T) {
	expectNoErrors(t, `
module main
enum Color { red, green, blue }
`)
}

func TestCheckEnumUsedAsFieldType(t *testing.T) {
	expectNoErrors(t, `
module main
enum State { on, off }
type Switch {
    state: State
}
`)
}

// Func declarations
// -----------------------------------------------------------------------------

func TestCheckFuncDecl(t *testing.T) {
	expectNoErrors(t, `
module main
func greet(name: string) string {
    return "hello"
}
`)
}

func TestCheckFuncWithOptionalParam(t *testing.T) {
	expectNoErrors(t, `
module main
func f(x: string, y: int?) int {
    return 42
}
`)
}

// Decl declarations
// -----------------------------------------------------------------------------

func TestCheckDeclDecl(t *testing.T) {
	expectNoErrors(t, `
module main
import "std"
import "std/posix"

decl create_user(name: string) std.Step {
    posix.dir { path = "/home/${self.name}" }
}
`)
}

func TestCheckDeclStub(t *testing.T) {
	expectNoErrors(t, `
module main
import "std"
decl my_step(x: string, y: int) std.Step
`)
}

// Let bindings
// -----------------------------------------------------------------------------

func TestCheckLetBinding(t *testing.T) {
	expectNoErrors(t, `
module main
let x = 42
`)
}

// Type resolution
// -----------------------------------------------------------------------------

func TestResolveBuiltinTypes(t *testing.T) {
	c := New(nil)
	cases := []struct {
		name string
		want Type
	}{
		{"string", StringType},
		{"int", IntType},
		{"bool", BoolType},
		{"any", AnyType},
	}
	for _, tc := range cases {
		name := &ast.DottedName{Parts: []*ast.Ident{{Name: tc.name}}}
		nt := &ast.NamedType{Name: name}
		got := c.resolveType(nt)
		if got != tc.want {
			t.Errorf("resolveType(%q): got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsAssignableTo(t *testing.T) {
	cases := []struct {
		name string
		src  Type
		dst  Type
		want bool
	}{
		{"same type", StringType, StringType, true},
		{"different types", StringType, IntType, false},
		{"T to T?", StringType, &Optional{Inner: StringType}, true},
		{"none to T?", NoneType, &Optional{Inner: StringType}, true},
		{"none to T", NoneType, StringType, false},
		{"anything to any", IntType, AnyType, true},
		{"T to different T?", IntType, &Optional{Inner: StringType}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsAssignableTo(tc.src, tc.dst)
			if got != tc.want {
				t.Errorf("IsAssignableTo(%v, %v): got %v, want %v",
					tc.src, tc.dst, got, tc.want)
			}
		})
	}
}

// Scope
// -----------------------------------------------------------------------------

func TestScopeLookup(t *testing.T) {
	parent := NewScope(nil, ScopeFile)
	parent.Define(&Symbol{Name: "x", Type: IntType, Kind: SymLet})
	child := NewScope(parent, ScopeBlock)
	child.Define(&Symbol{Name: "y", Type: StringType, Kind: SymLet})

	if child.Lookup("y") == nil {
		t.Error("child should find y")
	}
	if child.Lookup("x") == nil {
		t.Error("child should find x in parent")
	}
	if parent.Lookup("y") != nil {
		t.Error("parent should NOT find y")
	}
}

func TestScopeShadowing(t *testing.T) {
	parent := NewScope(nil, ScopeFile)
	parent.Define(&Symbol{Name: "x", Type: IntType, Kind: SymLet})
	child := NewScope(parent, ScopeBlock)
	child.Define(&Symbol{Name: "x", Type: StringType, Kind: SymLet})

	sym := child.Lookup("x")
	if sym == nil || sym.Type != StringType {
		t.Error("child should shadow parent's x")
	}
}

func TestScopeDuplicateInSameScope(t *testing.T) {
	s := NewScope(nil, ScopeFile)
	s.Define(&Symbol{Name: "x", Type: IntType, Kind: SymLet})
	ok := s.Define(&Symbol{Name: "x", Type: StringType, Kind: SymLet})
	if ok {
		t.Error("duplicate define should return false")
	}
}

func TestScopeMutability(t *testing.T) {
	file := NewScope(nil, ScopeFile)
	fn := NewScope(file, ScopeFunc)
	block := NewScope(fn, ScopeBlock)
	step := NewScope(file, ScopeDecl)

	if file.AllowsMutation() {
		t.Error("file scope should not allow mutation")
	}
	if !fn.AllowsMutation() {
		t.Error("func scope should allow mutation")
	}
	if !block.AllowsMutation() {
		t.Error("block inside func should allow mutation")
	}
	if step.AllowsMutation() {
		t.Error("step scope should not allow mutation")
	}
}
