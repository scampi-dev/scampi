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
import "std/local"
let vps = local.target { name = "dev" }
let d = std.deploy(name = "web", targets = [vps])
d { posix.dir { path = "/tmp/test" } }
`)
}

func TestCheckBlockExprInline(t *testing.T) {
	expectNoErrors(t, `
module main
import "std"
import "std/posix"
import "std/local"

std.deploy(name = "web", targets = [local.target { name = "dev" }]) {
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
import "std/local"

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

// Attribute types and binding
// -----------------------------------------------------------------------------

func TestCheckAttrTypeMarker(t *testing.T) {
	expectNoErrors(t, `
module main
type @nonempty {}

func f(@nonempty name: string) string
`)
}

func TestCheckAttrTypeUnknown(t *testing.T) {
	expectError(t, `
module main

func f(@undefined name: string) string
`, "unknown attribute: @undefined")
}

func TestCheckAttrTypeMarkerRejectsArgs(t *testing.T) {
	expectError(t, `
module main
type @nonempty {}

func f(@nonempty("oops") name: string) string
`, "marker attribute @nonempty takes no arguments")
}

func TestCheckAttrTypeSinglePositional(t *testing.T) {
	expectNoErrors(t, `
module main
type @since { version: string }

func f(@since("0.5") name: string) string
`)
}

func TestCheckAttrTypeSinglePositionalNamedForm(t *testing.T) {
	expectNoErrors(t, `
module main
type @since { version: string }

func f(@since(version="0.5") name: string) string
`)
}

func TestCheckAttrTypeWrongArgType(t *testing.T) {
	expectError(t, `
module main
type @since { version: string }

func f(@since(42) name: string) string
`, "cannot bind int to attribute @since")
}

func TestCheckAttrTypeUnknownNamedArg(t *testing.T) {
	expectError(t, `
module main
type @since { version: string }

func f(@since(zzz="0.5") name: string) string
`, "attribute @since has no field zzz")
}

func TestCheckAttrTypeMissingRequired(t *testing.T) {
	expectError(t, `
module main
type @since { version: string }

func f(@since name: string) string
`, "attribute @since missing required field version")
}

func TestCheckAttrTypeWithDefault(t *testing.T) {
	expectNoErrors(t, `
module main
type @deprecated { message: string = "" }

func f(@deprecated name: string) string
`)
}

func TestCheckAttrTypeVariadicMultiple(t *testing.T) {
	expectNoErrors(t, `
module main
type @oneof { values: list[string] }

func f(@oneof("present", "absent", "latest") state: string) string
`)
}

func TestCheckAttrTypeVariadicSingle(t *testing.T) {
	// Single positional with a list field should also be variadic-bound.
	expectNoErrors(t, `
module main
type @oneof { values: list[string] }

func f(@oneof("only") state: string) string
`)
}

func TestCheckAttrTypeVariadicAsList(t *testing.T) {
	// A list literal as the single positional binds directly.
	expectNoErrors(t, `
module main
type @oneof { values: list[string] }

func f(@oneof(["a", "b"]) state: string) string
`)
}

func TestCheckAttrTypeVariadicWrongElementType(t *testing.T) {
	expectError(t, `
module main
type @oneof { values: list[string] }

func f(@oneof("ok", 42, "also") state: string) string
`, "cannot bind int")
}

func TestCheckAttrTypeMultiFieldAllNamed(t *testing.T) {
	expectNoErrors(t, `
module main
type @path {
  absolute:   bool = false
  must_exist: bool = false
}

func f(@path(absolute=true, must_exist=true) p: string) string
`)
}

func TestCheckAttrTypeMultiFieldFirstPositional(t *testing.T) {
	expectNoErrors(t, `
module main
type @path {
  absolute:   bool = false
  must_exist: bool = false
}

func f(@path(true, must_exist=true) p: string) string
`)
}

func TestCheckAttrTypeMultiFieldTooManyPositionals(t *testing.T) {
	expectError(t, `
module main
type @path {
  absolute:   bool = false
  must_exist: bool = false
}

func f(@path(true, true) p: string) string
`, "accepts at most one positional argument")
}

func TestCheckAttrTypePositionalAndNamedSameField(t *testing.T) {
	expectError(t, `
module main
type @path {
  absolute:   bool = false
  must_exist: bool = false
}

func f(@path(true, absolute=true) p: string) string
`, "already bound by positional")
}

func TestCheckAttrTypeStackedAttrs(t *testing.T) {
	expectNoErrors(t, `
module main
type @nonempty {}
type @path { absolute: bool = false }

func f(
    @nonempty
    @path(absolute=true)
    p: string,
) string
`)
}

func TestCheckAttrTypeOnStructField(t *testing.T) {
	expectNoErrors(t, `
module main
type @nonempty {}

type User {
    @nonempty
    name: string
}
`)
}

func TestCheckAttrTypeOnDeclParam(t *testing.T) {
	expectNoErrors(t, `
module main
type Step
type @nonempty {}

decl posix.copy(
    @nonempty
    src: string,
) Step
`)
}

func TestCheckAttrTypeFieldDefCarriesAttributes(t *testing.T) {
	// After checking, the FuncType for a function whose params carry
	// attributes should expose those attributes on the corresponding
	// FieldDef.Attributes slice, qualified by the declaring module.
	src := `
module main
type @secretkey {}

func secret(@secretkey name: string) string
`
	l := lex.New("test.scampi", []byte(src))
	p := parse.New(l)
	f := p.Parse()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	c := New(nil)
	c.Check(f)
	if errs := c.Errors(); len(errs) > 0 {
		t.Fatalf("check errors: %v", errs)
	}
	sym := c.FileScope().Lookup("secret")
	if sym == nil {
		t.Fatal("secret symbol not found")
	}
	ft, ok := sym.Type.(*FuncType)
	if !ok {
		t.Fatalf("expected FuncType, got %T", sym.Type)
	}
	if len(ft.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(ft.Params))
	}
	if len(ft.Params[0].Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %d", len(ft.Params[0].Attributes))
	}
	if ft.Params[0].Attributes[0].QualifiedName != "main.@secretkey" {
		t.Errorf("attr qualified name: got %q, want %q",
			ft.Params[0].Attributes[0].QualifiedName, "main.@secretkey")
	}
}

func TestCheckAttrTypeNamespacedUnknown(t *testing.T) {
	// std has no attribute types registered yet (Stage 4 territory),
	// so this exercises the dotted-name resolution path: it should
	// emit a clean "unknown attribute" diagnostic, not crash.
	expectError(t, `
module main
import "std"

func f(@std.secret name: string) string
`, "unknown attribute: @std.secret")
}

// UFCS — `x.f(args)` desugars to `f(x, args)`
// -----------------------------------------------------------------------------

// TestUFCSBasic — a free function whose first param matches the
// receiver's type can be called as a method on the receiver.
func TestUFCSBasic(t *testing.T) {
	expectNoErrors(t, `
module main

func double(n: int) int {
  return n
}

func test() int {
  let x = 5
  return x.double()
}
`)
}

// TestUFCSWithExtraArgs — UFCS calls forward additional arguments
// past the receiver to the function's remaining parameters.
func TestUFCSWithExtraArgs(t *testing.T) {
	expectNoErrors(t, `
module main

func clamp(n: int, lo: int, hi: int) int {
  return n
}

func test() int {
  let x = 5
  return x.clamp(0, 10)
}
`)
}

// TestUFCSChained — chained UFCS calls work because each call's
// return value becomes the receiver of the next call.
func TestUFCSChained(t *testing.T) {
	expectNoErrors(t, `
module main

func double(n: int) int {
  return n
}

func inc(n: int) int {
  return n
}

func test() int {
  let x = 5
  return x.double().inc()
}
`)
}

// TestUFCSReceiverTypeMismatch — if the function's first param type
// doesn't accept the receiver's type, UFCS resolution fails. The
// fallback path then errors via the standard "no field" message
// because the selector is not a valid field access either.
func TestUFCSReceiverTypeMismatch(t *testing.T) {
	expectError(t, `
module main

func double(n: int) int {
  return n
}

func test() int {
  let s = "hello"
  return s.double()
}
`, "cannot access .double on string")
}

// TestUFCSFunctionNotInScope — if the named function doesn't exist
// in scope at all, the existing "no field" path fires.
func TestUFCSFunctionNotInScope(t *testing.T) {
	expectError(t, `
module main

func test() int {
  let x = 5
  return x.nonexistent()
}
`, "cannot access .nonexistent on int")
}

// TestUFCSDoesNotShadowModuleAccess — `posix.copy(...)` is a module
// member call, not UFCS. The Tier 1 (import-namespace) path runs
// before any UFCS attempt.
func TestUFCSDoesNotShadowModuleAccess(t *testing.T) {
	expectNoErrors(t, `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "h" }

std.deploy(name = "t", targets = [host]) {
  posix.dir { path = "/etc/foo" }
}
`)
}

// TestUFCSImportedModuleFunction — UFCS resolves through an
// imported module's free functions. `(5).range()` dispatches to
// `std.range(5)` because `std` is imported and `std.range`'s first
// parameter accepts an int.
func TestUFCSImportedModuleFunction(t *testing.T) {
	expectNoErrors(t, `
module main
import "std"

let zero_to_4 = (5).range()
`)
}

// TestUFCSImportedModuleNotImported — without `import "std"`, the
// `range` function isn't reachable and `(5).range()` errors via the
// standard "no field" path. Confirms imports are gated.
func TestUFCSImportedModuleNotImported(t *testing.T) {
	expectError(t, `
module main

let x = (5).range()
`, "cannot access .range on int")
}

// TestUFCSLocalShadowsImported — a local function with the same
// name as an imported function takes precedence over the import.
// This mirrors normal lexical-scope shadowing rules.
func TestUFCSLocalShadowsImported(t *testing.T) {
	expectNoErrors(t, `
module main
import "std"

func range(n: int) int {
  return n
}

// Calls the local range, not std.range. The local returns int, so
// type-checking the binding as int proves the local won.
let x: int = (5).range()
`)
}

// TestUFCSAmbiguousAcrossModules — when two imported modules both
// expose a function with the same name and a matching first param,
// the checker emits an ambiguity error listing all candidates.
//
// Constructed in-memory because no two stdlib modules currently
// have a name collision; this exercises the resolution rule
// directly via synthetic module scopes.
func TestUFCSAmbiguousAcrossModules(t *testing.T) {
	// Build two synthetic modules each with a `length(s: string) int`.
	intRet := IntType
	mkLengthScope := func() *Scope {
		s := NewScope(nil, ScopeFile)
		s.Define(&Symbol{
			Name: "length",
			Kind: SymFunc,
			Type: &FuncType{
				Params: []*FieldDef{{Name: "s", Type: StringType}},
				Ret:    intRet,
			},
		})
		return s
	}
	modules := map[string]*Scope{
		"a": mkLengthScope(),
		"b": mkLengthScope(),
	}

	src := `module main
import "a"
import "b"

let n = "hello".length()
`
	l := lex.New("test.scampi", []byte(src))
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		t.Fatalf("lex: %v", errs)
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	c := New(modules)
	c.Check(f)
	errs := c.Errors()
	if len(errs) == 0 {
		t.Fatal("expected ambiguity error, got none")
	}
	found := false
	for _, e := range errs {
		if contains(e.Msg, "ambiguous UFCS") &&
			contains(e.Msg, "a.length") &&
			contains(e.Msg, "b.length") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ambiguous UFCS error mentioning both candidates, got: %v", errs)
	}
}

// TestUFCSStructFieldBeatsUFCS — when a struct has a function-typed
// field, the field-access path wins over the UFCS fallback. (Scampi
// doesn't have user-defined function-typed fields today, so this
// just verifies that adding a free function doesn't accidentally
// shadow existing field access semantics.)
func TestUFCSDoesNotConflictWithExistingFields(t *testing.T) {
	// Posix.PkgState is an enum; .present is a variant access, not
	// a UFCS call. Adding a free function called `present` in scope
	// should not break the existing access.
	expectNoErrors(t, `
module main
import "std"
import "std/posix"

func present(s: string) string {
  return s
}

func test() posix.PkgState {
  return posix.PkgState.present
}
`)
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
	if !step.AllowsMutation() {
		t.Error("decl scope should allow mutation")
	}
}

func TestDeclBodyAllowsMutation(t *testing.T) {
	expectNoErrors(t, `
module main

func helper() string { return "x" }

decl build(name: string) string {
  let m = {"key": "val"}
  m["extra"] = name
  return helper()
}
`)
}

func TestFileScopeBlocksMutation(t *testing.T) {
	expectError(t, `
module main

let m = {"key": "val"}
m["extra"] = "nope"
`, "mutation not allowed")
}

// Duplicate field declarations
// -----------------------------------------------------------------------------

func TestDuplicateFieldInTypeDecl(t *testing.T) {
	expectError(t, `module main
type X {
  name: string
  name: string
}
`, "duplicate field: name")
}

func TestDuplicateFieldInAttrTypeDecl(t *testing.T) {
	expectError(t, `module main
type @tag {
  value: string
  value: string
}
`, "duplicate field: value")
}

func TestNoDuplicateFieldInTypeDecl(t *testing.T) {
	expectNoErrors(t, `module main
type X {
  name: string
  age: int
}
`)
}

// Control-flow analysis — all paths must return
// -----------------------------------------------------------------------------

func TestNotAllPathsReturn_IfWithoutElse(t *testing.T) {
	expectError(t, `
module main
type X { name: string }
func b(flag: bool) X {
  if flag {
    return X { name = "hello" }
  }
}
`, "not all paths return a value")
}

func TestAllPathsReturn_IfElse(t *testing.T) {
	expectNoErrors(t, `
module main
type X { name: string }
func b(flag: bool) X {
  if flag {
    return X { name = "hello" }
  } else {
    return X { name = "world" }
  }
}
`)
}

func TestAllPathsReturn_Simple(t *testing.T) {
	expectNoErrors(t, `
module main
type X { name: string }
func b() X {
  return X { name = "hello" }
}
`)
}

func TestNotAllPathsReturn_EmptyBody(t *testing.T) {
	expectError(t, `
module main
func f() int {
}
`, "not all paths return a value")
}

func TestNotAllPathsReturn_NestedIfMissingElse(t *testing.T) {
	expectError(t, `
module main
func f(a: bool, b: bool) int {
  if a {
    if b {
      return 1
    } else {
      return 2
    }
  }
}
`, "not all paths return a value")
}

func TestAllPathsReturn_NestedIfElse(t *testing.T) {
	expectNoErrors(t, `
module main
func f(a: bool, b: bool) int {
  if a {
    if b {
      return 1
    } else {
      return 2
    }
  } else {
    return 3
  }
}
`)
}

func TestNotAllPathsReturn_ForDoesNotCount(t *testing.T) {
	expectError(t, `
module main
func f(xs: list[int]) int {
  for x in xs {
    return x
  }
}
`, "not all paths return a value")
}

func TestAllPathsReturn_ReturnAfterFor(t *testing.T) {
	expectNoErrors(t, `
module main
func f(xs: list[int]) int {
  for x in xs {
  }
  return 0
}
`)
}

// Pub visibility
// -----------------------------------------------------------------------------

func TestPubLetParsesClean(t *testing.T) {
	expectNoErrors(t, `
module main
pub let x = 42
`)
}

func TestPubFuncParsesClean(t *testing.T) {
	expectNoErrors(t, `
module main
pub func f() int {
  return 1
}
`)
}

func TestPubTypeParsesClean(t *testing.T) {
	expectNoErrors(t, `
module main
pub type Config {
  name: string
}
`)
}

func TestPubEnumParsesClean(t *testing.T) {
	expectNoErrors(t, `
module main
pub enum State { on, off }
`)
}

func TestPubOnNonDecl(t *testing.T) {
	l := lex.New("test.scampi", []byte("module main\npub 42\n"))
	p := parse.New(l)
	_ = p.Parse()
	errs := p.Errors()
	if len(errs) == 0 {
		t.Fatal("expected parse error for pub on non-declaration")
	}
	found := false
	for _, e := range errs {
		if contains(e.Msg, "pub must be followed by") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'pub must be followed by' error, got: %v", errs)
	}
}

func TestPubSetsPublicOnAST(t *testing.T) {
	l := lex.New("test.scampi", []byte(`
module main
pub let x = 1
let y = 2
pub func f() int { return 1 }
func g() int { return 2 }
pub type T { name: string }
type U { name: string }
pub enum E { a, b }
enum F { c, d }
`))
	p := parse.New(l)
	f := p.Parse()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}

	checks := []struct {
		name   string
		public bool
	}{
		{"x", true}, {"y", false},
		{"f", true}, {"g", false},
		{"T", true}, {"U", false},
		{"E", true}, {"F", false},
	}

	declIdx := 0
	for _, d := range f.Decls {
		var name string
		var public bool
		switch d := d.(type) {
		case *ast.LetDecl:
			name, public = d.Name.Name, d.Public
		case *ast.FuncDecl:
			name, public = d.Name.Name, d.Public
		case *ast.TypeDecl:
			name, public = d.Name.Name, d.Public
		case *ast.EnumDecl:
			name, public = d.Name.Name, d.Public
		default:
			continue
		}
		if declIdx >= len(checks) {
			break
		}
		exp := checks[declIdx]
		if name != exp.name {
			t.Errorf("[%d] expected name %q, got %q", declIdx, exp.name, name)
		}
		if public != exp.public {
			t.Errorf("[%d] %s: expected Public=%v, got %v", declIdx, name, exp.public, public)
		}
		declIdx++
	}
}

func TestPublicView_FiltersPrivateSymbols(t *testing.T) {
	s := NewScope(nil, ScopeFile)
	s.Define(&Symbol{Name: "exported", Kind: SymFunc, IsPublic: true})
	s.Define(&Symbol{Name: "private", Kind: SymFunc, IsPublic: false})
	s.Define(&Symbol{Name: "also_pub", Kind: SymLet, IsPublic: true})

	pub := s.PublicView()

	if pub.Lookup("exported") == nil {
		t.Error("exported symbol should be visible in PublicView")
	}
	if pub.Lookup("also_pub") == nil {
		t.Error("also_pub symbol should be visible in PublicView")
	}
	if pub.Lookup("private") != nil {
		t.Error("private symbol should NOT be visible in PublicView")
	}
	if len(pub.Symbols()) != 2 {
		t.Errorf("expected 2 public symbols, got %d", len(pub.Symbols()))
	}
}

func TestPubVisibility_PrivateFuncNotAccessible(t *testing.T) {
	// Build a module "helpers" with one pub func and one private func.
	modSrc := `
module helpers
pub func visible() int { return 1 }
func hidden() int { return 2 }
`
	ml := lex.New("helpers.scampi", []byte(modSrc))
	mp := parse.New(ml)
	mf := mp.Parse()
	if errs := mp.Errors(); len(errs) > 0 {
		t.Fatalf("module parse errors: %v", errs)
	}

	modules, err := BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	// Check the module to get its scope.
	mc := New(modules)
	mc.Check(mf)
	if errs := mc.Errors(); len(errs) > 0 {
		t.Fatalf("module check errors: %v", errs)
	}

	// Export only public symbols — simulates what linker/usermod does.
	modules["helpers"] = mc.FileScope().PublicView()

	// Now check a consumer file that imports "helpers".
	consumerSrc := `
module main
import "helpers"
let a = helpers.visible()
`
	cc := New(modules)
	cf := parseFile(t, consumerSrc)
	cc.Check(cf)
	if errs := cc.Errors(); len(errs) > 0 {
		t.Fatalf("expected no errors accessing pub func, got: %v", errs)
	}

	// Accessing the private func should fail.
	consumerSrc2 := `
module main
import "helpers"
let b = helpers.hidden()
`
	cc2 := New(modules)
	cf2 := parseFile(t, consumerSrc2)
	cc2.Check(cf2)
	errs2 := cc2.Errors()
	if len(errs2) == 0 {
		t.Fatal("expected error accessing private func, got none")
	}
}

func TestPubVisibility_PubFuncReturningPrivateType(t *testing.T) {
	modSrc := `
module helpers
type Internal { name: string }
pub func make() Internal { return Internal { name = "ok" } }
`
	modules, err := BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	mc := New(modules)
	mf := parseFile(t, modSrc)
	mc.Check(mf)
	if errs := mc.Errors(); len(errs) > 0 {
		t.Fatalf("module check errors: %v", errs)
	}
	modules["helpers"] = mc.FileScope().PublicView()

	// Calling the pub func that returns a private type should work.
	consumerSrc := `
module main
import "helpers"
let x = helpers.make()
`
	cc := New(modules)
	cf := parseFile(t, consumerSrc)
	cc.Check(cf)
	if errs := cc.Errors(); len(errs) > 0 {
		t.Fatalf("expected no errors calling pub func returning private type, got: %v", errs)
	}

	// But constructing the private type directly should fail.
	consumerSrc2 := `
module main
import "helpers"
let x = helpers.Internal { name = "nope" }
`
	cc2 := New(modules)
	cf2 := parseFile(t, consumerSrc2)
	cc2.Check(cf2)
	if errs := cc2.Errors(); len(errs) == 0 {
		t.Fatal("expected error constructing private type directly, got none")
	}
}

func parseFile(t *testing.T, src string) *ast.File {
	t.Helper()
	l := lex.New("test.scampi", []byte(src))
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		t.Fatalf("lex errors: %v", errs)
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return f
}
