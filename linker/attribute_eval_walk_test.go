// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/std"
)

// TestEvalWalk_LetBoundIntViolatesMax proves the walker validates
// let-bound ints via the resolved value rather than rejecting the
// non-literal Ident. The AST-walker can't see past the Ident; the
// eval-walker resolves p=70000 and dispatches @max(65535).
func TestEvalWalk_LetBoundIntViolatesMax(t *testing.T) {
	src := `module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "host" }
let p = 70000

std.deploy(name = "main", targets = [host]) {
  posix.firewall { port = p }
}
`
	fileScope, modules, result := evalForAttrTest(t, src)
	err := runAttributeEvalChecks(result, []byte(src), "test.scampi", fileScope, modules, DefaultAttributes())
	if err == nil {
		t.Fatal("expected eval-walker to emit @max violation for let-bound port=70000")
	}
	var diags diagnostic.Diagnostics
	assertDiagnosticsFor(t, err, "port", &diags)
}

// TestEvalWalk_LetBoundValidPasses proves the walker emits nothing
// when the resolved value satisfies all attributes — exactly the
// regression that issue-184 is fixing for real configs.
func TestEvalWalk_LetBoundValidPasses(t *testing.T) {
	src := `module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "host" }
let p = 22

std.deploy(name = "main", targets = [host]) {
  posix.firewall { port = p }
}
`
	fileScope, modules, result := evalForAttrTest(t, src)
	err := runAttributeEvalChecks(result, []byte(src), "test.scampi", fileScope, modules, DefaultAttributes())
	if err != nil {
		t.Fatalf("expected no diagnostics for let-bound port=22, got: %v", err)
	}
}

// TestEvalWalk_LiteralViolatesMax proves the walker behaves the same
// as the AST-walker for literal arguments: it validates and emits the
// same violation.
func TestEvalWalk_LiteralViolatesMax(t *testing.T) {
	src := `module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "host" }

std.deploy(name = "main", targets = [host]) {
  posix.firewall { port = 70000 }
}
`
	fileScope, modules, result := evalForAttrTest(t, src)
	err := runAttributeEvalChecks(result, []byte(src), "test.scampi", fileScope, modules, DefaultAttributes())
	if err == nil {
		t.Fatal("expected eval-walker to emit @max violation for port=70000")
	}
	var diags diagnostic.Diagnostics
	assertDiagnosticsFor(t, err, "port", &diags)
}

// TestEvalWalk_SkipsCrossStepRef proves the walker silently skips
// RefVal arguments rather than dispatching attribute behaviours
// against a runtime-only handle. Ref values resolve only at engine
// time; static checks defer to the runtime.
func TestEvalWalk_SkipsCrossStepRef(t *testing.T) {
	// Build a minimal DeclType with one annotated param, then dispatch
	// directly with a RefVal Field — bypassing scope lookup so we
	// test only the per-StructVal dispatch logic.
	dt := &check.DeclType{
		Name: "fake_decl",
		Params: []*check.FieldDef{
			{
				Name: "port",
				Attributes: []check.ResolvedAttribute{
					{QualifiedName: "std.@min", Args: map[string]any{"value": int64(1024)}},
				},
			},
		},
	}
	sv := &eval.StructVal{
		TypeName: "fake_decl",
		QualName: "fake_decl",
		Fields:   map[string]eval.Value{"port": &eval.RefVal{}},
	}
	ctx := &linkContext{}
	dispatchEvalAttributes(ctx, sv, dt, DefaultAttributes(), nil, "test.scampi")
	if len(ctx.diags) != 0 {
		t.Errorf("expected no diagnostics for RefVal, got %d", len(ctx.diags))
	}
}

// evalForAttrTest runs the lang pipeline on a snippet and returns the
// pieces needed to call runAttributeEvalChecks directly.
func evalForAttrTest(t *testing.T, src string) (*check.Scope, map[string]*check.Scope, *eval.Result) {
	t.Helper()
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	data := []byte(src)
	l := lex.New("test.scampi", data)
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		t.Fatalf("lex: %v", errs)
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	c := check.New(modules)
	c.Check(f)
	if errs := c.Errors(); len(errs) > 0 {
		t.Fatalf("check: %v", errs)
	}
	r, evalErrs := eval.Eval(f, data, eval.WithStubs(std.FS))
	if len(evalErrs) > 0 {
		t.Fatalf("eval: %v", evalErrs)
	}
	return c.FileScope(), modules, r
}

// assertDiagnosticsFor unwraps an attribute walker error and confirms
// at least one diagnostic mentions the named param.
func assertDiagnosticsFor(t *testing.T, err error, paramName string, out *diagnostic.Diagnostics) {
	t.Helper()
	diags, ok := err.(diagnostic.Diagnostics)
	if !ok {
		t.Fatalf("expected diagnostic.Diagnostics, got %T: %v", err, err)
	}
	*out = diags
	for _, d := range diags {
		if dErr, ok := d.(*attrDocError); ok && dErr.Param == paramName {
			return
		}
	}
	t.Errorf("no diagnostic mentions param %q; got %d diagnostics", paramName, len(diags))
}
