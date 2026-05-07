// SPDX-License-Identifier: GPL-3.0-only

package eval

import (
	"testing"

	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/std"
)

func evalSrc(t *testing.T, src string) *Result {
	t.Helper()
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	l := lex.New("test.scampi", []byte(src))
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		t.Fatalf("lex errors: %v", errs)
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	c := check.New(modules)
	c.Check(f)
	if errs := c.Errors(); len(errs) > 0 {
		t.Fatalf("check errors: %v", errs)
	}
	r, errs := Eval(f, []byte(src), WithStubs(std.FS))
	if len(errs) > 0 {
		t.Fatalf("eval errors: %v", errs)
	}
	return r
}

// findByRetType returns all StructVals with the given return type
// from both Bindings and Exprs.
func findByRetType(r *Result, retType string) []*StructVal {
	var found []*StructVal
	for _, v := range r.Bindings {
		if sv, ok := v.(*StructVal); ok && sv.RetType == retType {
			found = append(found, sv)
		}
	}
	for _, v := range r.Exprs {
		if sv, ok := v.(*StructVal); ok && sv.RetType == retType {
			found = append(found, sv)
		}
	}
	return found
}

// Let bindings
// -----------------------------------------------------------------------------

func TestEvalLetString(t *testing.T) {
	r := evalSrc(t, `
module main
let x = "hello"
`)
	_ = r
}

func TestEvalLetInt(t *testing.T) {
	r := evalSrc(t, `
module main
let x = 42
`)
	_ = r
}

func TestEvalLetArithmetic(t *testing.T) {
	r := evalSrc(t, `
module main
let a = 10
let b = 3
let sum = a + b
let prod = a * b
`)
	_ = r
}

// String interpolation
// -----------------------------------------------------------------------------

func TestEvalStringInterp(t *testing.T) {
	r := evalSrc(t, `
module main
let name = "world"
let greeting = "hello ${name}!"
`)
	_ = r
}

// List and map literals
// -----------------------------------------------------------------------------

func TestEvalList(t *testing.T) {
	r := evalSrc(t, `
module main
let xs = [1, 2, 3]
`)
	_ = r
}

func TestEvalMap(t *testing.T) {
	r := evalSrc(t, `
module main
let m = {"a": 1, "b": 2}
`)
	_ = r
}

// Struct literal
// -----------------------------------------------------------------------------

func TestEvalStructLit(t *testing.T) {
	r := evalSrc(t, `
module main
type User {
  name: string
  admin: bool = false
}
let u = User { name = "alice" }
`)
	sv, ok := r.Bindings["u"].(*StructVal)
	if !ok {
		t.Fatal("u is not a StructVal")
	}
	if sv.Fields["name"].(*StringVal).V != "alice" {
		t.Error("name should be alice")
	}
	bv, ok := sv.Fields["admin"].(*BoolVal)
	if !ok {
		t.Fatal("admin field missing or wrong type")
	}
	if bv.V != false {
		t.Error("admin default should be false")
	}
}

func TestEvalTypeFieldDefaultOverride(t *testing.T) {
	r := evalSrc(t, `
module main
type Item {
  name:  string
  count: int = 1
}
let a = Item { name = "x" }
let b = Item { name = "y", count = 5 }
`)
	a := r.Bindings["a"].(*StructVal)
	if a.Fields["count"].(*IntVal).V != 1 {
		t.Error("a.count should default to 1")
	}
	b := r.Bindings["b"].(*StructVal)
	if b.Fields["count"].(*IntVal).V != 5 {
		t.Error("b.count should be 5 (explicit override)")
	}
}

func TestEvalTypeFieldDefaultDotAccess(t *testing.T) {
	r := evalSrc(t, `
module main
type Box {
  label: string
  size:  int = 10
}
let b = Box { label = "test" }
let s = b.size
`)
	iv, ok := r.Bindings["s"].(*IntVal)
	if !ok {
		t.Fatal("s should be an IntVal")
	}
	if iv.V != 10 {
		t.Error("b.size should be 10 from default")
	}
}

func TestEvalTypeFieldDefaultInLoop(t *testing.T) {
	r := evalSrc(t, `
module main
type Entry {
  id:    int
  cores: int    = 1
  mem:   string = "512M"
}
let items = [
  Entry { id = 1, cores = 4 },
  Entry { id = 2 },
]
let c0 = items[0].cores
let c1 = items[1].cores
let m1 = items[1].mem
`)
	if r.Bindings["c0"].(*IntVal).V != 4 {
		t.Error("c0 should be 4 (explicit)")
	}
	if r.Bindings["c1"].(*IntVal).V != 1 {
		t.Error("c1 should be 1 (default)")
	}
	if r.Bindings["m1"].(*StringVal).V != "512M" {
		t.Error("m1 should be 512M (default)")
	}
}

// Function calls
// -----------------------------------------------------------------------------

func TestEvalFuncCall(t *testing.T) {
	r := evalSrc(t, `
module main
func add(a: int, b: int) int {
  return a + b
}
let result = add(1, 2)
`)
	_ = r
}

// Secret resolvers
// -----------------------------------------------------------------------------

func TestEvalSecretResolver(t *testing.T) {
	src := `
module main
import "std/secrets"
let resolver = secrets.from_file(path = "secrets.json")
`
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	l := lex.New("test.scampi", []byte(src))
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		t.Fatalf("lex errors: %v", errs)
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	c := check.New(modules)
	c.Check(f)
	if errs := c.Errors(); len(errs) > 0 {
		t.Fatalf("check errors: %v", errs)
	}
	stubBackend := "stub-backend"
	r, errs := Eval(f, []byte(src),
		WithStubs(std.FS),
		WithBuiltinFunc("secrets.from_file", func(_ []Value, _ map[string]Value) (Value, string) {
			return &OpaqueVal{TypeName: "SecretResolver", Inner: stubBackend}, ""
		}),
	)
	if len(errs) > 0 {
		t.Fatalf("eval errors: %v", errs)
	}
	val, ok := r.Bindings["resolver"]
	if !ok {
		t.Fatal("expected binding 'resolver'")
	}
	rv, ok := val.(*OpaqueVal)
	if !ok {
		t.Fatalf("expected *OpaqueVal, got %T", val)
	}
	if rv.Inner != stubBackend {
		t.Errorf("expected stub backend, got %v", rv.Inner)
	}
}

// Target
// -----------------------------------------------------------------------------

func TestEvalTarget(t *testing.T) {
	src := `
module main
import "std"
import "std/posix"
import "std/ssh"
let vps = ssh.target { name = "vps", host = "10.0.0.1", user = "root" }
`
	r := evalSrc(t, src)
	targets := findByRetType(r, "Target")
	if len(targets) != 1 {
		t.Fatalf("expected 1 Target, got %d", len(targets))
	}
	sv := targets[0]
	if sv.QualName != "ssh.target" {
		t.Errorf("target type: got %q, want %q", sv.QualName, "ssh.target")
	}
	if n, ok := sv.Fields["name"].(*StringVal); !ok || n.V != "vps" {
		t.Errorf("target name: got %v", sv.Fields["name"])
	}
}

// Matchers
// -----------------------------------------------------------------------------

// Matcher constructors are stub funcs returning the opaque
// `Matcher` type. The default stub-func code path lifts each call
// into a StructVal with TypeName = the func name and Fields = the
// kwargs. The verifier (in target/test/) reads those two fields to
// dispatch by matcher kind. This test pins that contract — if it
// breaks, the verifier breaks too.
func TestEvalMatchers(t *testing.T) {
	src := `
module main
import "std/posix"
import "std/test/matchers"

let m_exact   = matchers.has_exact_content("hello")
let m_sub     = matchers.has_substring("world")
let m_regex   = matchers.matches_regex("^foo")
let m_empty   = matchers.is_empty()
let m_present = matchers.is_present()
let m_absent  = matchers.is_absent()
let m_svc     = matchers.has_svc_status(posix.ServiceState.running)
let m_pkg     = matchers.has_pkg_status(posix.PkgState.present)
`
	r := evalSrc(t, src)

	for name, want := range map[string]struct {
		typeName string
		field    string
		val      string
	}{
		"m_exact":   {"has_exact_content", "content", "hello"},
		"m_sub":     {"has_substring", "substring", "world"},
		"m_regex":   {"matches_regex", "pattern", "^foo"},
		"m_empty":   {"is_empty", "", ""},
		"m_present": {"is_present", "", ""},
		"m_absent":  {"is_absent", "", ""},
	} {
		v, ok := r.Bindings[name]
		if !ok {
			t.Errorf("%s: missing from bindings", name)
			continue
		}
		sv, ok := v.(*StructVal)
		if !ok {
			t.Errorf("%s: got %T, want *StructVal", name, v)
			continue
		}
		if sv.TypeName != want.typeName {
			t.Errorf("%s: TypeName=%q, want %q", name, sv.TypeName, want.typeName)
		}
		if sv.RetType != "Matcher" {
			t.Errorf("%s: RetType=%q, want %q", name, sv.RetType, "Matcher")
		}
		if want.field != "" {
			s, ok := sv.Fields[want.field].(*StringVal)
			if !ok || s.V != want.val {
				t.Errorf("%s: Fields[%q]=%v, want %q",
					name, want.field, sv.Fields[want.field], want.val)
			}
		}
	}

	// Status matchers carry the enum variant value verbatim. The
	// evaluator currently represents enum values as StringVal of
	// the variant name — pin that so the verifier can read it.
	svc, ok := r.Bindings["m_svc"].(*StructVal)
	if !ok {
		t.Fatalf("m_svc: not a StructVal")
	}
	if svc.TypeName != "has_svc_status" || svc.RetType != "Matcher" {
		t.Errorf("m_svc: got %q/%q", svc.TypeName, svc.RetType)
	}
	if status, ok := svc.Fields["status"].(*StringVal); !ok || status.V != "running" {
		t.Errorf("m_svc: status=%v", svc.Fields["status"])
	}

	pkg, ok := r.Bindings["m_pkg"].(*StructVal)
	if !ok {
		t.Fatalf("m_pkg: not a StructVal")
	}
	if pkg.TypeName != "has_pkg_status" || pkg.RetType != "Matcher" {
		t.Errorf("m_pkg: got %q/%q", pkg.TypeName, pkg.RetType)
	}
	if status, ok := pkg.Fields["status"].(*StringVal); !ok || status.V != "present" {
		t.Errorf("m_pkg: status=%v", pkg.Fields["status"])
	}
}

// For loop
// -----------------------------------------------------------------------------

func TestEvalForLoop(t *testing.T) {
	r := evalSrc(t, `
module main
let xs = [1, 2, 3]
let doubled = [x * 2 for x in xs]
`)
	_ = r
}

// If expression
// -----------------------------------------------------------------------------

func TestEvalIfExpr(t *testing.T) {
	r := evalSrc(t, `
module main
let x = 10
let label = if x > 5 { "big" } else { "small" }
`)
	_ = r
}

// Boolean operators
// -----------------------------------------------------------------------------

func TestEvalBoolOps(t *testing.T) {
	r := evalSrc(t, `
module main
let a = true && false
let b = true || false
let c = !true
`)
	_ = r
}

// In operator
// -----------------------------------------------------------------------------

func TestEvalInOperator(t *testing.T) {
	r := evalSrc(t, `
module main
let xs = ["a", "b", "c"]
let found = "b" in xs
`)
	_ = r
}

// UFCS — runtime dispatch for `x.f(args)` resolves to `f(x, args)`
// when no field on x matches and a free function f exists.
// -----------------------------------------------------------------------------

func TestEvalUFCSBasic(t *testing.T) {
	r := evalSrc(t, `
module main

func double(n: int) int {
  return n + n
}

let result = (5).double()
`)
	v, ok := r.Bindings["result"]
	if !ok {
		t.Fatal("result binding missing")
	}
	iv, ok := v.(*IntVal)
	if !ok {
		t.Fatalf("expected IntVal, got %T", v)
	}
	if iv.V != 10 {
		t.Errorf("expected 10, got %d", iv.V)
	}
}

func TestEvalUFCSChained(t *testing.T) {
	r := evalSrc(t, `
module main

func inc(n: int) int {
  return n + 1
}

func double(n: int) int {
  return n + n
}

let result = (3).inc().double().inc()
`)
	v, ok := r.Bindings["result"]
	if !ok {
		t.Fatal("result binding missing")
	}
	iv, ok := v.(*IntVal)
	if !ok {
		t.Fatalf("expected IntVal, got %T", v)
	}
	// (3).inc() = 4 → .double() = 8 → .inc() = 9
	if iv.V != 9 {
		t.Errorf("expected 9, got %d", iv.V)
	}
}

func TestEvalUFCSWithExtraArgs(t *testing.T) {
	r := evalSrc(t, `
module main

func add(a: int, b: int) int {
  return a + b
}

let result = (10).add(32)
`)
	v, ok := r.Bindings["result"]
	if !ok {
		t.Fatal("result binding missing")
	}
	iv, ok := v.(*IntVal)
	if !ok {
		t.Fatalf("expected IntVal, got %T", v)
	}
	if iv.V != 42 {
		t.Errorf("expected 42, got %d", iv.V)
	}
}

// TestEvalUFCSDoesNotShadowModuleAccess — `posix.copy(...)` is a
// module member call. The Tier 1 path runs before any UFCS attempt.
// Intra-module pub visibility
// -----------------------------------------------------------------------------

// A pub func in a user module must be able to call non-pub helpers
// defined in the same module (same `module` declaration, possibly in a
// different file). Regression test for #209: registerUserModules was
// filtering non-pub symbols from the module map, which also feeds the
// intra-module sibling injection in callFunc.
func TestEvalPubFuncCallsNonPubHelper(t *testing.T) {
	// Simulate a two-file module: _index.scampi has a pub func that
	// calls a non-pub helper from api.scampi. We merge them into one
	// AST (same as loadMultiFileModule does) for simplicity.
	modSrc := `
module helpers
func internal_add(a: int, b: int) int {
  return a + b
}
pub func add(a: int, b: int) int {
  return internal_add(a, b)
}
`
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	ml := lex.New("helpers.scampi", []byte(modSrc))
	mp := parse.New(ml)
	mf := mp.Parse()
	if errs := ml.Errors(); len(errs) > 0 {
		t.Fatalf("module lex: %v", errs)
	}
	if errs := mp.Errors(); len(errs) > 0 {
		t.Fatalf("module parse: %v", errs)
	}
	mc := check.New(modules)
	mc.Check(mf)
	if errs := mc.Errors(); len(errs) > 0 {
		t.Fatalf("module check: %v", errs)
	}
	modules["helpers"] = mc.FileScope().PublicView()

	consumerSrc := `
module main
import "helpers"
let result = helpers.add(17, 25)
`
	cl := lex.New("test.scampi", []byte(consumerSrc))
	cp := parse.New(cl)
	cf := cp.Parse()
	if errs := cl.Errors(); len(errs) > 0 {
		t.Fatalf("consumer lex: %v", errs)
	}
	if errs := cp.Errors(); len(errs) > 0 {
		t.Fatalf("consumer parse: %v", errs)
	}
	cc := check.New(modules)
	cc.Check(cf)
	if errs := cc.Errors(); len(errs) > 0 {
		t.Fatalf("consumer check: %v", errs)
	}

	r, errs := Eval(cf, []byte(consumerSrc),
		WithStubs(std.FS),
		WithUserModules([]UserModule{{
			Name:   "helpers",
			File:   mf,
			Source: []byte(modSrc),
		}}),
	)
	if len(errs) > 0 {
		t.Fatalf("eval errors: %v", errs)
	}
	v, ok := r.Bindings["result"]
	if !ok {
		t.Fatal("result binding missing")
	}
	iv, ok := v.(*IntVal)
	if !ok {
		t.Fatalf("expected IntVal, got %T", v)
	}
	if iv.V != 42 {
		t.Errorf("expected 42, got %d", iv.V)
	}
}

// Sibling modules (same package, different file) inject functions
// directly into the top-level env via WithSiblingModules. Functions
// are callable by bare name without a module prefix.
func TestEvalSiblingModuleBareName(t *testing.T) {
	sibSrc := `
module helpers
func internal_mul(a: int, b: int) int {
  return a * b
}
`
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	sl := lex.New("sibling.scampi", []byte(sibSrc))
	sp := parse.New(sl)
	sf := sp.Parse()
	if errs := sp.Errors(); len(errs) > 0 {
		t.Fatalf("sibling parse: %v", errs)
	}
	sc := check.New(modules)
	sc.Check(sf)

	// The consumer is in the same module — calls by bare name.
	consumerSrc := `
module helpers
let result = internal_mul(6, 7)
`
	cl := lex.New("test.scampi", []byte(consumerSrc))
	cp := parse.New(cl)
	cf := cp.Parse()
	// Provide sibling scope to the checker so it sees internal_mul.
	cc := check.New(modules)
	cc.WithScope(sc.FileScope())
	cc.Check(cf)
	if errs := cc.Errors(); len(errs) > 0 {
		t.Fatalf("consumer check: %v", errs)
	}

	r, errs := Eval(cf, []byte(consumerSrc),
		WithStubs(std.FS),
		WithSiblingModules([]UserModule{{
			Name:   "helpers",
			File:   sf,
			Source: []byte(sibSrc),
		}}),
	)
	if len(errs) > 0 {
		t.Fatalf("eval errors: %v", errs)
	}
	v, ok := r.Bindings["result"]
	if !ok {
		t.Fatal("result binding missing")
	}
	iv, ok := v.(*IntVal)
	if !ok {
		t.Fatalf("expected IntVal, got %T", v)
	}
	if iv.V != 42 {
		t.Errorf("expected 42, got %d", iv.V)
	}
}

// External callers must NOT be able to access non-pub functions.
func TestEvalNonPubNotVisibleExternally(t *testing.T) {
	modSrc := `
module helpers
func hidden() int { return 99 }
pub func visible() int { return 1 }
`
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	ml := lex.New("helpers.scampi", []byte(modSrc))
	mp := parse.New(ml)
	mf := mp.Parse()
	if errs := mp.Errors(); len(errs) > 0 {
		t.Fatalf("module parse: %v", errs)
	}
	mc := check.New(modules)
	mc.Check(mf)
	if errs := mc.Errors(); len(errs) > 0 {
		t.Fatalf("module check: %v", errs)
	}
	modules["helpers"] = mc.FileScope().PublicView()

	// The checker should reject this — but even if it doesn't, the
	// evaluator's pub-only env map should not expose "hidden".
	consumerSrc := `
module main
import "helpers"
let result = helpers.visible()
`
	cl := lex.New("test.scampi", []byte(consumerSrc))
	cp := parse.New(cl)
	cf := cp.Parse()
	cc := check.New(modules)
	cc.Check(cf)
	if errs := cc.Errors(); len(errs) > 0 {
		t.Fatalf("consumer check: %v", errs)
	}

	r, errs := Eval(cf, []byte(consumerSrc),
		WithStubs(std.FS),
		WithUserModules([]UserModule{{
			Name:   "helpers",
			File:   mf,
			Source: []byte(modSrc),
		}}),
	)
	if len(errs) > 0 {
		t.Fatalf("eval errors: %v", errs)
	}
	v, ok := r.Bindings["result"]
	if !ok {
		t.Fatal("result binding missing")
	}
	iv, ok := v.(*IntVal)
	if !ok {
		t.Fatalf("expected IntVal, got %T", v)
	}
	if iv.V != 1 {
		t.Errorf("expected 1, got %d", iv.V)
	}
}

// UFCS — runtime dispatch
// -----------------------------------------------------------------------------

func TestEvalUFCSDoesNotShadowModuleAccess(t *testing.T) {
	r := evalSrc(t, `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "h" }

std.deploy(name = "t", targets = [host]) {
  posix.dir { path = "/etc/foo" }
}
`)
	if len(r.Exprs) == 0 && len(r.Bindings) == 0 {
		t.Fatal("expected at least the host binding and deploy expr")
	}
}

// Std module func bodies
// -----------------------------------------------------------------------------

func TestEvalStdJoin(t *testing.T) {
	r := evalSrc(t, `
module main
import "std"

let full = std.join(["a", "b", "c"], ";")
let single = std.join(["x"], "-")
`)
	if v, ok := r.Bindings["full"].(*StringVal); !ok || v.V != "a;b;c" {
		t.Fatalf("full: got %v, want 'a;b;c'", r.Bindings["full"])
	}
	if v, ok := r.Bindings["single"].(*StringVal); !ok || v.V != "x" {
		t.Fatalf("single: got %v, want 'x'", r.Bindings["single"])
	}
}

// Struct indexing
// -----------------------------------------------------------------------------

func TestEvalStructIndexAccess(t *testing.T) {
	r := evalSrc(t, `
module main

let s = { name = "alice", age = 30 }
let n = s["name"]
let a = s["age"]
let missing = s["nope"]
`)
	if v, ok := r.Bindings["n"].(*StringVal); !ok || v.V != "alice" {
		t.Fatalf("expected n = alice, got %v", r.Bindings["n"])
	}
	if v, ok := r.Bindings["a"].(*IntVal); !ok || v.V != 30 {
		t.Fatalf("expected a = 30, got %v", r.Bindings["a"])
	}
	if _, ok := r.Bindings["missing"].(*NoneVal); !ok {
		t.Fatalf("expected missing = none, got %T", r.Bindings["missing"])
	}
}

// Regression: bare identifier access to a user-module `pub let`
// returned the raw ThunkVal without forcing. Downstream consumers
// (linker → evalToGo → switch on type) didn't recognize ThunkVal
// and fell through to nil, silently turning list-valued state
// fields into Go nil. Symptom in the wild: rest.resource drift
// comparator saw "[..] vs null" and fired PUT every run.
func TestEvalIdentForcesThunk(t *testing.T) {
	modSrc := `
module helpers
pub let xs = ["a", "b", "c"]
pub func get_xs() list[string] {
  return xs
}
`
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	ml := lex.New("helpers.scampi", []byte(modSrc))
	mp := parse.New(ml)
	mf := mp.Parse()
	mc := check.New(modules)
	mc.Check(mf)
	if errs := mc.Errors(); len(errs) > 0 {
		t.Fatalf("module check: %v", errs)
	}
	modules["helpers"] = mc.FileScope().PublicView()

	consumerSrc := `
module main
import "helpers"
let result = helpers.get_xs()
`
	cl := lex.New("test.scampi", []byte(consumerSrc))
	cp := parse.New(cl)
	cf := cp.Parse()
	cc := check.New(modules)
	cc.Check(cf)
	if errs := cc.Errors(); len(errs) > 0 {
		t.Fatalf("consumer check: %v", errs)
	}

	r, errs := Eval(cf, []byte(consumerSrc),
		WithStubs(std.FS),
		WithUserModules([]UserModule{{
			Name:   "helpers",
			File:   mf,
			Source: []byte(modSrc),
		}}),
	)
	if len(errs) > 0 {
		t.Fatalf("eval errors: %v", errs)
	}

	v, ok := r.Bindings["result"]
	if !ok {
		t.Fatal("result binding missing")
	}
	// Without the fix, `xs` resolves to a ThunkVal inside get_xs()
	// and the function returns it as-is. With the fix, the bare
	// ident lookup forces, so result is the underlying ListVal.
	list, ok := v.(*ListVal)
	if !ok {
		t.Fatalf("result must be ListVal (thunk should be forced at access), got %T", v)
	}
	if len(list.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list.Items))
	}
	for i, want := range []string{"a", "b", "c"} {
		s, ok := list.Items[i].(*StringVal)
		if !ok || s.V != want {
			t.Errorf("item[%d] = %v, want %q", i, list.Items[i], want)
		}
	}
}

func TestEvalUnique(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "strings",
			src:  `let result = std.unique(["a", "b", "a", "c", "b", "a"])`,
			want: []string{"a", "b", "c"},
		},
		{
			name: "ints",
			src:  `let result = std.unique([1, 2, 1, 3, 2, 1])`,
			want: []string{"1", "2", "3"},
		},
		{
			name: "empty",
			src:  `let result = std.unique([])`,
			want: nil,
		},
		{
			name: "preserves first-occurrence order",
			src:  `let result = std.unique(["c", "a", "b", "a", "c"])`,
			want: []string{"c", "a", "b"},
		},
		{
			name: "no duplicates passes through",
			src:  `let result = std.unique(["x", "y", "z"])`,
			want: []string{"x", "y", "z"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := evalSrc(t, "module main\nimport \"std\"\n"+tc.src)
			v, ok := r.Bindings["result"].(*ListVal)
			if !ok {
				t.Fatalf("expected ListVal, got %T", r.Bindings["result"])
			}
			if len(v.Items) != len(tc.want) {
				t.Fatalf("len = %d, want %d (items=%v)", len(v.Items), len(tc.want), v.Items)
			}
			for i, want := range tc.want {
				switch got := v.Items[i].(type) {
				case *StringVal:
					if got.V != want {
						t.Errorf("item[%d] = %q, want %q", i, got.V, want)
					}
				case *IntVal:
					if intStr(got.V) != want {
						t.Errorf("item[%d] = %d, want %s", i, got.V, want)
					}
				default:
					t.Errorf("item[%d] unexpected type %T", i, got)
				}
			}
		})
	}
}

func intStr(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+(n%10))) + out
		n /= 10
	}
	if neg {
		out = "-" + out
	}
	return out
}

// Probe: does Color resolve INSIDE a func body (not as default)?
// If this passes but the default-eval test fails, the bug is in
// the default-eval scope specifically, not module exposure.
func TestEvalUserModuleEnumInBody(t *testing.T) {
	modSrc := `
module paint
pub enum Color { red, green, blue }
pub func get_red() string {
  return "${Color.red}"
}
`
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	ml := lex.New("paint.scampi", []byte(modSrc))
	mp := parse.New(ml)
	mf := mp.Parse()
	mc := check.New(modules)
	mc.Check(mf)
	if errs := mc.Errors(); len(errs) > 0 {
		t.Fatalf("module check: %v", errs)
	}
	modules["paint"] = mc.FileScope().PublicView()

	consumerSrc := `
module main
import "paint"
let result = paint.get_red()
`
	cl := lex.New("test.scampi", []byte(consumerSrc))
	cp := parse.New(cl)
	cf := cp.Parse()
	cc := check.New(modules)
	cc.Check(cf)
	if errs := cc.Errors(); len(errs) > 0 {
		t.Fatalf("consumer check: %v", errs)
	}

	r, errs := Eval(
		cf,
		[]byte(consumerSrc),
		WithStubs(std.FS),
		WithUserModules([]UserModule{{Name: "paint", File: mf, Source: []byte(modSrc)}}),
	)
	if len(errs) > 0 {
		t.Fatalf("eval errors: %v", errs)
	}
	v, ok := r.Bindings["result"].(*StringVal)
	if !ok {
		t.Fatalf("expected StringVal, got %T", r.Bindings["result"])
	}
	if v.V != "red" {
		t.Errorf("expected 'red', got %q", v.V)
	}
}

// Regression: parameter defaults that reference module-private names
// (enums, helpers, pub lets) must evaluate in the *defining* module's
// scope, not the caller's. Otherwise users have to re-import every
// internal name a default touches, defeating encapsulation.
func TestEvalDefaultEvalsInDefiningModuleScope(t *testing.T) {
	modSrc := `
module paint
pub enum Color { red, green, blue }
pub let default_palette = [Color.red, Color.green]
pub func make_palette(palette: list[Color] = default_palette) list[Color] {
  return palette
}
`
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	ml := lex.New("paint.scampi", []byte(modSrc))
	mp := parse.New(ml)
	mf := mp.Parse()
	mc := check.New(modules)
	mc.Check(mf)
	if errs := mc.Errors(); len(errs) > 0 {
		t.Fatalf("module check: %v", errs)
	}
	modules["paint"] = mc.FileScope().PublicView()

	consumerSrc := `
module main
import "paint"
let result = paint.make_palette()
`
	cl := lex.New("test.scampi", []byte(consumerSrc))
	cp := parse.New(cl)
	cf := cp.Parse()
	cc := check.New(modules)
	cc.Check(cf)
	if errs := cc.Errors(); len(errs) > 0 {
		t.Fatalf("consumer check: %v", errs)
	}

	r, errs := Eval(cf, []byte(consumerSrc),
		WithStubs(std.FS),
		WithUserModules([]UserModule{{
			Name:   "paint",
			File:   mf,
			Source: []byte(modSrc),
		}}),
	)
	if len(errs) > 0 {
		t.Fatalf("eval errors: %v", errs)
	}

	v, ok := r.Bindings["result"]
	if !ok {
		t.Fatal("result binding missing")
	}
	list, ok := v.(*ListVal)
	if !ok {
		t.Fatalf("result must be ListVal, got %T (%v)", v, v)
	}
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 items (default palette), got %d: %v", len(list.Items), list.Items)
	}
}

func TestEvalStructIndexInComprehension(t *testing.T) {
	r := evalSrc(t, `
module main

let people = [
  { name = "alice", role = "admin" },
  { name = "bob", role = "user" },
]
let names = [p["name"] for p in people]
`)
	v, ok := r.Bindings["names"].(*ListVal)
	if !ok {
		t.Fatalf("expected ListVal, got %T", r.Bindings["names"])
	}
	if len(v.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(v.Items))
	}
	if s, ok := v.Items[0].(*StringVal); !ok || s.V != "alice" {
		t.Fatalf("expected alice, got %v", v.Items[0])
	}
	if s, ok := v.Items[1].(*StringVal); !ok || s.V != "bob" {
		t.Fatalf("expected bob, got %v", v.Items[1])
	}
}
