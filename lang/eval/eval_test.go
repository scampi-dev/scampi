// SPDX-License-Identifier: GPL-3.0-only

package eval

import (
	"testing"

	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
)

func evalSrc(t *testing.T, src string) *Result {
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
	c := check.New()
	c.Check(f)
	if errs := c.Errors(); len(errs) > 0 {
		t.Fatalf("check errors: %v", errs)
	}
	r, errs := Eval(f, []byte(src))
	if len(errs) > 0 {
		t.Fatalf("eval errors: %v", errs)
	}
	return r
}

// Let bindings
// -----------------------------------------------------------------------------

func TestEvalLetString(t *testing.T) {
	r := evalSrc(t, `let x = "hello"`)
	_ = r
}

func TestEvalLetInt(t *testing.T) {
	r := evalSrc(t, `let x = 42`)
	_ = r
}

func TestEvalLetArithmetic(t *testing.T) {
	src := `
let a = 10
let b = 3
let sum = a + b
let prod = a * b
`
	r := evalSrc(t, src)
	_ = r
}

// String interpolation
// -----------------------------------------------------------------------------

func TestEvalStringInterp(t *testing.T) {
	src := `
let name = "world"
let greeting = "hello ${name}!"
`
	r := evalSrc(t, src)
	_ = r
}

// List and map literals
// -----------------------------------------------------------------------------

func TestEvalList(t *testing.T) {
	src := `let xs = [1, 2, 3]`
	r := evalSrc(t, src)
	_ = r
}

func TestEvalMap(t *testing.T) {
	src := `let m = {"a": 1, "b": 2}`
	r := evalSrc(t, src)
	_ = r
}

// Struct literal
// -----------------------------------------------------------------------------

func TestEvalStructLit(t *testing.T) {
	src := `
struct User {
  name: string
  admin: bool = false
}
let u = User { name = "alice" }
`
	r := evalSrc(t, src)
	_ = r
}

// Function calls
// -----------------------------------------------------------------------------

func TestEvalFuncCall(t *testing.T) {
	src := `
func add(a: int, b: int) int {
  return a + b
}
let result = add(1, 2)
`
	r := evalSrc(t, src)
	_ = r
}

// Secrets config
// -----------------------------------------------------------------------------

func TestEvalSecrets(t *testing.T) {
	src := `
import "std"
std.secrets { backend = "file", path = "secrets.json" }
`
	r := evalSrc(t, src)
	if r.Secrets == nil {
		t.Fatal("expected SecretsConfig")
	}
	if r.Secrets.Backend != "file" {
		t.Errorf("backend: got %q, want %q", r.Secrets.Backend, "file")
	}
	if r.Secrets.Path != "secrets.json" {
		t.Errorf("path: got %q, want %q", r.Secrets.Path, "secrets.json")
	}
}

// Target
// -----------------------------------------------------------------------------

func TestEvalTarget(t *testing.T) {
	src := `
import "std"
import "std/target"
let vps = target.ssh { name = "vps", host = "10.0.0.1", user = "root" }
`
	r := evalSrc(t, src)
	if len(r.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(r.Targets))
	}
	if r.Targets[0].Name != "vps" {
		t.Errorf("target name: got %q", r.Targets[0].Name)
	}
	if r.Targets[0].Kind != "ssh" {
		t.Errorf("target kind: got %q", r.Targets[0].Kind)
	}
}

// For loop
// -----------------------------------------------------------------------------

func TestEvalForLoop(t *testing.T) {
	src := `
let xs = [1, 2, 3]
let doubled = [x * 2 for x in xs]
`
	r := evalSrc(t, src)
	_ = r
}

// If expression
// -----------------------------------------------------------------------------

func TestEvalIfExpr(t *testing.T) {
	src := `
let x = 10
let label = if x > 5 { "big" } else { "small" }
`
	r := evalSrc(t, src)
	_ = r
}

// Boolean operators
// -----------------------------------------------------------------------------

func TestEvalBoolOps(t *testing.T) {
	src := `
let a = true && false
let b = true || false
let c = !true
`
	r := evalSrc(t, src)
	_ = r
}

// In operator
// -----------------------------------------------------------------------------

func TestEvalInOperator(t *testing.T) {
	src := `
let xs = ["a", "b", "c"]
let found = "b" in xs
`
	r := evalSrc(t, src)
	_ = r
}
