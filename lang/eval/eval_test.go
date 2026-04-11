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
	_ = r
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

// Secrets config
// -----------------------------------------------------------------------------

func TestEvalSecrets(t *testing.T) {
	src := `
module main
import "std"
std.secrets { backend = std.SecretsBackend.file, path = "secrets.json" }
`
	r := evalSrc(t, src)
	secrets := findByRetType(r, "SecretsConfig")
	if len(secrets) != 1 {
		t.Fatalf("expected 1 SecretsConfig, got %d", len(secrets))
	}
	sv := secrets[0]
	if b, ok := sv.Fields["backend"].(*StringVal); !ok || b.V != "file" {
		t.Errorf("backend: got %v", sv.Fields["backend"])
	}
	if p, ok := sv.Fields["path"].(*StringVal); !ok || p.V != "secrets.json" {
		t.Errorf("path: got %v", sv.Fields["path"])
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
