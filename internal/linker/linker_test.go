// SPDX-License-Identifier: GPL-3.0-only

package linker_test

import (
	"context"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/lang/check"
	"scampi.dev/scampi/internal/lang/eval"
	"scampi.dev/scampi/internal/lang/lex"
	"scampi.dev/scampi/internal/lang/parse"
	"scampi.dev/scampi/internal/linker"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/std"
	"scampi.dev/scampi/test/harness"
)

func TestLinkBasicDeploy(t *testing.T) {
	src := `
module main
import "std"
import "std/posix"
import "std/ssh"
import "std/local"

let vps = ssh.target { name = "vps", host = "10.0.0.1", user = "root" }

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

func TestLoadConfig_ParseErrorDiagnostic(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte("module main\n@@@ garbage\n")
	reg := engine.NewRegistry()

	capture := &harness.Capture{}
	_, err := linker.LoadConfig(diagnostic.NewCtx(context.Background(), capture), "/config.scampi", src, reg)
	if err == nil {
		t.Fatal("expected error for broken syntax")
	}

	tmpl := firstDiagnosticTemplate(t, capture)
	if tmpl.Source == nil {
		t.Fatal("diagnostic should have source span")
	}
	if tmpl.Source.StartLine == 0 {
		t.Error("diagnostic source span should have non-zero line")
	}
}

// firstDiagnosticTemplate returns the template of the first diagnostic
// raised through capture. Every linker diagnostic is now delivered via
// the emitter, so the error chain carries only sentinel values.
func firstDiagnosticTemplate(t *testing.T, capture *harness.Capture) event.Template {
	t.Helper()
	for _, ev := range capture.Events {
		switch v := ev.(type) {
		case event.Error:
			return v.Template
		case event.Warning:
			return v.Template
		case event.Info:
			return v.Template
		}
	}
	t.Fatal("no diagnostic raised through capture")
	return event.Template{}
}

func TestLoadConfig_SecretErrorDiagnostic(t *testing.T) {
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(`module main
import "std"
import "std/posix"
import "std/ssh"
import "std/local"
let host = local.target { name = "local" }
let x = std.secret("missing_key")
std.deploy(name = "test", targets = [host]) {
  posix.dir { path = "/tmp/test" }
}
`)
	reg := engine.NewRegistry()

	capture := &harness.Capture{}
	_, err := linker.LoadConfig(diagnostic.NewCtx(context.Background(), capture), "/config.scampi", src, reg)
	if err == nil {
		t.Fatal("expected error for secret() without backend")
	}

	tmpl := firstDiagnosticTemplate(t, capture)
	if tmpl.Source == nil {
		t.Fatal("diagnostic should have source span")
	}
	if tmpl.Source.StartLine != 7 {
		t.Errorf("expected error at line 7 (secret call), got line %d", tmpl.Source.StartLine)
	}
}

func TestLinkPopulatesSpans(t *testing.T) {
	src := `module main
import "std"
import "std/posix"
import "std/ssh"

let vps = ssh.target { name = "vps", host = "10.0.0.1", user = "root" }

std.deploy(name = "web", targets = [vps]) {
  posix.dir { path = "/var/www" }
}
`
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
	reg := engine.NewRegistry()
	cfg, err := linker.Link(r, reg, "test.scampi", linker.WithSource(data))
	if err != nil {
		t.Fatalf("link: %v", err)
	}

	// Target spans
	ti, ok := cfg.Targets["vps"]
	if !ok {
		t.Fatal("target 'vps' not found")
	}
	if ti.Source.StartLine == 0 {
		t.Errorf("target Source.StartLine: got 0, want non-zero")
	}
	if ti.Source.Filename != "test.scampi" {
		t.Errorf("target Source.Filename: got %q, want %q", ti.Source.Filename, "test.scampi")
	}
	if fs, ok := ti.Fields["host"]; !ok {
		t.Error("target Fields[host]: missing")
	} else if fs.Value.StartLine == 0 {
		t.Errorf("target Fields[host].Value.StartLine: got 0, want non-zero")
	}

	// Step spans
	if len(cfg.Deploy) == 0 || len(cfg.Deploy[0].Steps) == 0 {
		t.Fatal("no steps in deploy")
	}
	step := cfg.Deploy[0].Steps[0]
	if step.Source.StartLine == 0 {
		t.Errorf("step Source.StartLine: got 0, want non-zero")
	}
	if fs, ok := step.Fields["path"]; !ok {
		t.Error("step Fields[path]: missing")
	} else if fs.Value.StartLine == 0 {
		t.Errorf("step Fields[path].Value.StartLine: got 0, want non-zero")
	}
}

// TestLinkZeroSpansWithoutSource verifies that without WithSource(),
// linked instances carry zero-valued spans — the back-compat path for
// callers that don't plumb source bytes.
func TestLinkZeroSpansWithoutSource(t *testing.T) {
	cfg := evalAndLink(t, `module main
import "std"
import "std/posix"
import "std/ssh"

let vps = ssh.target { name = "vps", host = "10.0.0.1", user = "root" }

std.deploy(name = "web", targets = [vps]) {
  posix.dir { path = "/var/www" }
}
`)
	if cfg.Targets["vps"].Source.StartLine != 0 {
		t.Errorf("expected zero StartLine without WithSource, got %d",
			cfg.Targets["vps"].Source.StartLine)
	}
	if len(cfg.Targets["vps"].Fields) != 0 {
		t.Errorf("expected nil Fields without WithSource, got %d entries",
			len(cfg.Targets["vps"].Fields))
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
