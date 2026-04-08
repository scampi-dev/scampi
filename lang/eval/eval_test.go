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
let vps = posix.ssh { name = "vps", host = "10.0.0.1", user = "root" }
`
	r := evalSrc(t, src)
	targets := findByRetType(r, "Target")
	if len(targets) != 1 {
		t.Fatalf("expected 1 Target, got %d", len(targets))
	}
	sv := targets[0]
	if sv.TypeName != "ssh" {
		t.Errorf("target type: got %q, want %q", sv.TypeName, "ssh")
	}
	if n, ok := sv.Fields["name"].(*StringVal); !ok || n.V != "vps" {
		t.Errorf("target name: got %v", sv.Fields["name"])
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
