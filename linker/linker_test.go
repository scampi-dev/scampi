// SPDX-License-Identifier: GPL-3.0-only

package linker_test

import (
	"testing"

	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/linker"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/std"
)

func TestLinkBasicDeploy(t *testing.T) {
	src := `
module main
import "std"
import "std/posix"

let vps = posix.ssh { name = "vps", host = "10.0.0.1", user = "root" }

std.deploy(name = "web", targets = [vps]) {
  posix.pkg { packages = ["nginx"], source = posix.pkg_system {} }
  posix.dir { path = "/var/www" }
}
`
	cfg := evalAndLink(t, src)

	if len(cfg.Targets) != 1 {
		t.Fatalf("targets: got %d, want 1", len(cfg.Targets))
	}
	ti, ok := cfg.Targets["vps"]
	if !ok {
		t.Fatal("target 'vps' not found")
	}
	if ti.Type.Kind() != "ssh" {
		t.Errorf("target kind: got %q, want %q", ti.Type.Kind(), "ssh")
	}

	if len(cfg.Deploy) != 1 {
		t.Fatalf("deploys: got %d, want 1", len(cfg.Deploy))
	}
	db := cfg.Deploy[0]
	if db.Name != "web" {
		t.Errorf("deploy name: got %q, want %q", db.Name, "web")
	}
	if len(db.Steps) != 2 {
		t.Fatalf("steps: got %d, want 2", len(db.Steps))
	}
	if db.Steps[0].Type.Kind() != "pkg" {
		t.Errorf("step 0 kind: got %q, want %q", db.Steps[0].Type.Kind(), "pkg")
	}
	if db.Steps[1].Type.Kind() != "dir" {
		t.Errorf("step 1 kind: got %q, want %q", db.Steps[1].Type.Kind(), "dir")
	}
}

func TestLinkUnresolvedStep(t *testing.T) {
	result := &eval.Result{
		Exprs: []eval.Value{
			&eval.BlockResultVal{
				TypeName: "Deploy",
				FuncName: "deploy",
				Fields:   map[string]eval.Value{"name": &eval.StringVal{V: "test"}},
				Body: []eval.Value{
					&eval.StructVal{
						TypeName: "nonexistent_step",
						RetType:  "Step",
						Fields:   map[string]eval.Value{},
					},
				},
			},
		},
	}
	reg := engine.NewRegistry()
	_, err := linker.Link(result, reg, "test.scampi")
	if err == nil {
		t.Fatal("expected link error for unresolved step")
	}
	ue, ok := err.(*linker.UnresolvedError)
	if !ok {
		t.Fatalf("expected *linker.UnresolvedError, got %T: %v", err, err)
	}
	if ue.Kind != "step" || ue.Name != "nonexistent_step" {
		t.Errorf("error: got kind=%q name=%q, want step/nonexistent_step", ue.Kind, ue.Name)
	}
}

func TestLinkUnresolvedTarget(t *testing.T) {
	result := &eval.Result{
		Bindings: map[string]eval.Value{
			"bad": &eval.StructVal{
				TypeName: "nonexistent_target",
				RetType:  "Target",
				Fields:   map[string]eval.Value{"name": &eval.StringVal{V: "bad"}},
			},
		},
	}
	reg := engine.NewRegistry()
	_, err := linker.Link(result, reg, "test.scampi")
	if err == nil {
		t.Fatal("expected link error for unresolved target")
	}
	ue, ok := err.(*linker.UnresolvedError)
	if !ok {
		t.Fatalf("expected *linker.UnresolvedError, got %T: %v", err, err)
	}
	if ue.Kind != "target" || ue.Name != "nonexistent_target" {
		t.Errorf("error: got kind=%q name=%q, want target/nonexistent_target", ue.Kind, ue.Name)
	}
}

func evalAndLink(t *testing.T, src string) spec.Config {
	t.Helper()
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	l := lex.New("test.scampi", []byte(src))
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
	r, errs := eval.Eval(f, []byte(src), eval.WithStubs(std.FS))
	if len(errs) > 0 {
		t.Fatalf("eval: %v", errs)
	}
	reg := engine.NewRegistry()
	cfg, err := linker.Link(r, reg, "test.scampi")
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	return cfg
}
