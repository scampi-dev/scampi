// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"io/fs"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/test/harness"
)

// Test_Template_InspectSrcFile verifies template steps are inspectable with src files.
func Test_Template_InspectSrcFile(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "inspect-src"
    src = posix.source_local { path = "/tmpl.txt" }
    dest = "/out.txt"
    data = {
      "values": {
        "name": "world",
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/tmpl.txt"] = []byte("Hello, {{.name}}!")
	src.Files["/config.scampi"] = []byte(cfgStr)
	tgt.Files["/out.txt"] = []byte("old content")

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	result, err := e.InspectDiffFile(diagnostic.NewCtx(ctx, em), "")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.DestPath != "/out.txt" {
		t.Errorf("DestPath = %q, want %q", result.DestPath, "/out.txt")
	}
	if got := string(result.Desired); got != "Hello, world!" {
		t.Errorf("Desired = %q, want %q", got, "Hello, world!")
	}
	if got := string(result.Current); got != "old content" {
		t.Errorf("Current = %q, want %q", got, "old content")
	}
}

// Test_Template_InspectInline verifies template steps are inspectable with inline content.
func Test_Template_InspectInline(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "inspect-inline"
    src = posix.source_inline { content = "Port: {{.port}}" }
    dest = "/app.conf"
    data = {
      "values": {
        "port": "8080",
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	result, err := e.InspectDiffFile(diagnostic.NewCtx(ctx, em), "")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if result.DestPath != "/app.conf" {
		t.Errorf("DestPath = %q, want %q", result.DestPath, "/app.conf")
	}
	if got := string(result.Desired); got != "Port: 8080" {
		t.Errorf("Desired = %q, want %q", got, "Port: 8080")
	}
	if result.Current != nil {
		t.Errorf("Current = %q, want nil (file doesn't exist)", result.Current)
	}
}

// Test_Template_BasicRender verifies basic template rendering with values.
func Test_Template_BasicRender(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "render-test"
    src = posix.source_local { path = "/tmpl.txt" }
    dest = "/out.txt"
    data = {
      "values": {
        "name": "world",
        "count": 42,
      },
    }
    perm = "0644"
    owner = "testuser"
    group = "testgroup"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/tmpl.txt"] = []byte("Hello, {{.name}}! Count: {{.count}}")
	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Verify rendered content
	data, ok := tgt.Files["/out.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	expected := "Hello, world! Count: 42"
	if string(data) != expected {
		t.Errorf("unexpected content: got %q, want %q", data, expected)
	}

	// Verify permissions
	mode, ok := tgt.Modes["/out.txt"]
	if !ok {
		t.Fatal("mode not set")
	}
	if mode != fs.FileMode(0o644) {
		t.Errorf("unexpected mode: got %o, want %o", mode, 0o644)
	}

	// Verify ownership
	owner, ok := tgt.Owners["/out.txt"]
	if !ok {
		t.Fatal("owner not set")
	}
	if owner.User != "testuser" || owner.Group != "testgroup" {
		t.Errorf("unexpected owner: got %+v, want user=testuser group=testgroup", owner)
	}
}

// Test_Template_InlineContent verifies template rendering with inline content.
func Test_Template_InlineContent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "inline-template"
    src = posix.source_inline { content = "Inline: {{.msg}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "msg": "hello",
      },
    }
    perm = "0600"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	data, ok := tgt.Files["/out.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	expected := "Inline: hello"
	if string(data) != expected {
		t.Errorf("unexpected content: got %q, want %q", data, expected)
	}
}

// Test_Template_EnvOverride verifies env variables override values.
func Test_Template_EnvOverride(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "env-override"
    src = posix.source_inline { content = "Port: {{.port}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "port": "8080",
      },
      "env": {
        "MY_PORT": "port",
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)
	src.Env["MY_PORT"] = "9000" // Override via env

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	data, ok := tgt.Files["/out.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	// Env var should override the default value
	expected := "Port: 9000"
	if string(data) != expected {
		t.Errorf("unexpected content: got %q, want %q", data, expected)
	}
}

// Test_Template_EnvNotSetUsesDefault verifies default is used when env not set.
func Test_Template_EnvNotSetUsesDefault(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "env-default"
    src = posix.source_inline { content = "Port: {{.port}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "port": "8080",
      },
      "env": {
        "MY_PORT": "port",
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource() // No env vars
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	data, ok := tgt.Files["/out.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	// Default value should be used since env var is not set
	expected := "Port: 8080"
	if string(data) != expected {
		t.Errorf("unexpected content: got %q, want %q", data, expected)
	}
}

// Test_Template_Idempotent verifies no changes when destination already matches.
func Test_Template_Idempotent(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "idempotent"
    src = posix.source_inline { content = "static content" }
    dest = "/out.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	// Pre-populate target with matching state
	tgt.Files["/out.txt"] = []byte("static content")
	tgt.Modes["/out.txt"] = fs.FileMode(0o644)
	tgt.Owners["/out.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Idempotent run: no Change events should fire.
	for _, c := range rec.Changes {
		t.Errorf("unexpected Change event: phase=%v op=%s", c.Phase, c.DisplayID)
	}
}

// Test_Template_ContentChange verifies changes are applied when content differs.
func Test_Template_ContentChange(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "content-change"
    src = posix.source_inline { content = "new content" }
    dest = "/out.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	// Pre-populate with different content
	tgt.Files["/out.txt"] = []byte("old content")
	tgt.Modes["/out.txt"] = fs.FileMode(0o644)
	tgt.Owners["/out.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Verify content was updated
	if string(tgt.Files["/out.txt"]) != "new content" {
		t.Errorf("content not updated: got %q, want %q", tgt.Files["/out.txt"], "new content")
	}

	executed := 0
	for _, c := range rec.Changes {
		if c.Phase == event.ChangeExecuted {
			executed++
		}
	}
	if executed == 0 {
		t.Error("expected executed changes due to content update")
	}
}

// Test_Template_ErrorParseError verifies template parse errors are reported.
func Test_Template_ErrorParseError(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "parse-error"
    src = posix.source_inline { content = "{{.unclosed" }
    dest = "/out.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err == nil {
		t.Fatal("expected error for parse failure, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.Diagnostics {
		if event.TemplateOf(d).ID == "step.template.Parse" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.Parse diagnostic, got: %s", rec)
	}
}

// Test_Template_ErrorExecError verifies template execution errors are reported.
func Test_Template_ErrorExecError(t *testing.T) {
	// Calls len on nil - triggers an exec error distinct from missingkey=error
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "exec-error"
    src = posix.source_inline { content = "{{len .missing}}" }
    dest = "/out.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err == nil {
		t.Fatal("expected error for exec failure, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.Diagnostics {
		if event.TemplateOf(d).ID == "step.template.Exec" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.Exec diagnostic, got: %s", rec)
	}
}

// Test_Template_ErrorSourceMissing verifies missing source file is reported.
func Test_Template_ErrorSourceMissing(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "source-missing"
    src = posix.source_local { path = "/nonexistent.txt" }
    dest = "/out.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)
	// Note: /nonexistent.txt is not added

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.Diagnostics {
		if event.TemplateOf(d).ID == "step.template.SourceMissing" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.SourceMissing diagnostic, got: %s", rec)
	}
}

// Test_Template_ErrorEnvKeyNotInValues verifies env key not in values is reported.
func Test_Template_ErrorEnvKeyNotInValues(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "env-key-missing"
    src = posix.source_inline { content = "{{.port}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "port": "8080",
      },
      "env": {
        "MY_HOST": "host",
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)
	src.Env["MY_HOST"] = "localhost" // Set the env var

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err == nil {
		t.Fatal("expected error for env key not in values, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.Diagnostics {
		if event.TemplateOf(d).ID == "step.template.EnvKeyNotInValues" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.EnvKeyNotInValues diagnostic, got: %s", rec)
	}
}

// Test_Template_ErrorDestDirMissing verifies missing dest directory is reported.
func Test_Template_ErrorDestDirMissing(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "dest-dir-missing"
    src = posix.source_inline { content = "content" }
    dest = "/nonexistent/dir/out.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err == nil {
		t.Fatal("expected error for missing dest directory, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.Diagnostics {
		if event.TemplateOf(d).ID == "step.template.DestDirMissing" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.DestDirMissing diagnostic, got: %s", rec)
	}
}

// Test_Template_ModeChange verifies mode changes are applied.
func Test_Template_ModeChange(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "mode-change"
    src = posix.source_inline { content = "content" }
    dest = "/out.txt"
    perm = "0755"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	// Pre-populate with wrong mode
	tgt.Files["/out.txt"] = []byte("content")
	tgt.Modes["/out.txt"] = fs.FileMode(0o644)
	tgt.Owners["/out.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Mode should be updated
	if tgt.Modes["/out.txt"] != fs.FileMode(0o755) {
		t.Errorf("mode not updated: got %o, want %o", tgt.Modes["/out.txt"], 0o755)
	}
}

// Test_Template_OwnerChange verifies owner changes are applied.
func Test_Template_OwnerChange(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "owner-change"
    src = posix.source_inline { content = "content" }
    dest = "/out.txt"
    perm = "0644"
    owner = "newuser"
    group = "newgroup"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	// Pre-populate with wrong owner
	tgt.Files["/out.txt"] = []byte("content")
	tgt.Modes["/out.txt"] = fs.FileMode(0o644)
	tgt.Owners["/out.txt"] = target.Owner{User: "olduser", Group: "oldgroup"}

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Owner should be updated
	owner := tgt.Owners["/out.txt"]
	if owner.User != "newuser" || owner.Group != "newgroup" {
		t.Errorf("owner not updated: got %+v, want user=newuser group=newgroup", owner)
	}
}

// Test_Template_MultipleValues verifies multiple values work correctly.
func Test_Template_MultipleValues(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "multi-values"
    src = posix.source_inline { content = "{{.host}}:{{.port}} - {{.name}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "host": "localhost",
        "port": 8080,
        "name": "myapp",
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	expected := "localhost:8080 - myapp"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// Test_Template_NoData verifies templates work without any data.
func Test_Template_NoData(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "no-data"
    src = posix.source_inline { content = "static template" }
    dest = "/out.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	expected := "static template"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// Test_Template_NestedValues verifies nested data structures work.
func Test_Template_NestedValues(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "nested"
    src = posix.source_inline { content = "{{.server.host}}:{{.server.port}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "server": {
          "host": "example.com",
          "port": 443,
        },
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	expected := "example.com:443"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// Test_Template_MultipleEnvOverrides verifies multiple env overrides work.
func Test_Template_MultipleEnvOverrides(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "multi-env"
    src = posix.source_inline { content = "{{.host}}:{{.port}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "host": "localhost",
        "port": "8080",
      },
      "env": {
        "MY_HOST": "host",
        "MY_PORT": "port",
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)
	src.Env["MY_HOST"] = "prod.example.com"
	src.Env["MY_PORT"] = "443"

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	expected := "prod.example.com:443"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// Test_Template_PartialEnvOverride verifies some env vars override while others use defaults.
func Test_Template_PartialEnvOverride(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "partial-env"
    src = posix.source_inline { content = "{{.host}}:{{.port}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "host": "localhost",
        "port": "8080",
      },
      "env": {
        "MY_HOST": "host",
        "MY_PORT": "port",
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.scampi"] = []byte(cfgStr)
	// Only set MY_HOST, not MY_PORT
	src.Env["MY_HOST"] = "prod.example.com"

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// MY_HOST overridden, MY_PORT uses default
	expected := "prod.example.com:8080"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// Test_Template_WriteFailure verifies write failure is handled.
func Test_Template_WriteFailure(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "write-fail"
    src = posix.source_inline { content = "content" }
    dest = "/out.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := harness.NewFaultyTarget(innerTgt)

	src.Files["/config.scampi"] = []byte(cfgStr)

	// Inject write failure
	tgt.InjectFault("WriteFile", "/out.txt", fs.ErrPermission)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := t.Context()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockDeclaredTarget(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))
	if err == nil {
		t.Fatal("expected error for write failure, got nil")
	}
}
