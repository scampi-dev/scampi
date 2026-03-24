// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"io/fs"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
)

// TestTemplate_Inspect_SrcFile verifies template steps are inspectable with src files.
func TestTemplate_Inspect_SrcFile(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="inspect-src",
            src=local("/tmpl.txt"),
            dest="/out.txt",
            data={
                "values": {
                    "name": "world",
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/tmpl.txt"] = []byte("Hello, {{.name}}!")
	src.Files["/config.star"] = []byte(cfgStr)
	tgt.Files["/out.txt"] = []byte("old content")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	result, err := e.InspectDiffFile(ctx, "")
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

// TestTemplate_Inspect_Inline verifies template steps are inspectable with inline content.
func TestTemplate_Inspect_Inline(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="inspect-inline",
            src=inline("Port: {{.port}}"),
            dest="/app.conf",
            data={
                "values": {
                    "port": "8080",
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer e.Close()

	result, err := e.InspectDiffFile(ctx, "")
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

// TestTemplate_BasicRender verifies basic template rendering with values.
func TestTemplate_BasicRender(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="render-test",
            src=local("/tmpl.txt"),
            dest="/out.txt",
            data={
                "values": {
                    "name": "world",
                    "count": 42,
                },
            },
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/tmpl.txt"] = []byte("Hello, {{.name}}! Count: {{.count}}")
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
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

// TestTemplate_InlineContent verifies template rendering with inline content.
func TestTemplate_InlineContent(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="inline-template",
            src=inline("Inline: {{.msg}}"),
            dest="/out.txt",
            data={
                "values": {
                    "msg": "hello",
                },
            },
            perm="0600",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
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

// TestTemplate_EnvOverride verifies env variables override values.
func TestTemplate_EnvOverride(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="env-override",
            src=inline("Port: {{.port}}"),
            dest="/out.txt",
            data={
                "values": {
                    "port": "8080",
                },
                "env": {
                    "MY_PORT": "port",
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)
	src.Env["MY_PORT"] = "9000" // Override via env

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
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

// TestTemplate_EnvNotSet_UsesDefault verifies default is used when env not set.
func TestTemplate_EnvNotSet_UsesDefault(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="env-default",
            src=inline("Port: {{.port}}"),
            dest="/out.txt",
            data={
                "values": {
                    "port": "8080",
                },
                "env": {
                    "MY_PORT": "port",
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource() // No env vars
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
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

// TestTemplate_Idempotent verifies no changes when destination already matches.
func TestTemplate_Idempotent(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="idempotent",
            src=inline("static content"),
            dest="/out.txt",
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	// Pre-populate target with matching state
	tgt.Files["/out.txt"] = []byte("static content")
	tgt.Modes["/out.txt"] = fs.FileMode(0o644)
	tgt.Owners["/out.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Check that all ops were skipped
	var actionFinished *event.ActionDetail
	for _, ev := range rec.actionEvents {
		if ev.Kind == event.ActionFinished {
			actionFinished = ev.Detail
			break
		}
	}

	if actionFinished == nil {
		t.Fatal("no ActionFinished event found")
	}

	if actionFinished.Summary.Skipped != actionFinished.Summary.Total {
		t.Errorf("expected all ops skipped: got %d/%d skipped",
			actionFinished.Summary.Skipped, actionFinished.Summary.Total)
	}
}

// TestTemplate_ContentChange verifies changes are applied when content differs.
func TestTemplate_ContentChange(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="content-change",
            src=inline("new content"),
            dest="/out.txt",
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	// Pre-populate with different content
	tgt.Files["/out.txt"] = []byte("old content")
	tgt.Modes["/out.txt"] = fs.FileMode(0o644)
	tgt.Owners["/out.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Verify content was updated
	if string(tgt.Files["/out.txt"]) != "new content" {
		t.Errorf("content not updated: got %q, want %q", tgt.Files["/out.txt"], "new content")
	}

	// Check that changes were made
	var actionFinished *event.ActionDetail
	for _, ev := range rec.actionEvents {
		if ev.Kind == event.ActionFinished {
			actionFinished = ev.Detail
			break
		}
	}

	if actionFinished == nil {
		t.Fatal("no ActionFinished event found")
	}

	if actionFinished.Summary.Changed == 0 {
		t.Error("expected changes due to content update")
	}
}

// TestTemplate_Error_ParseError verifies template parse errors are reported.
func TestTemplate_Error_ParseError(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="parse-error",
            src=inline("{{.unclosed"),
            dest="/out.txt",
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error for parse failure, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.opDiagnostics {
		if d.Detail.Template.ID == "builtin.template.Parse" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.Parse diagnostic, got: %s", rec)
	}
}

// TestTemplate_Error_ExecError verifies template execution errors are reported.
func TestTemplate_Error_ExecError(t *testing.T) {
	// Calls len on nil — triggers an exec error distinct from missingkey=error
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="exec-error",
            src=inline("{{len .missing}}"),
            dest="/out.txt",
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error for exec failure, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.opDiagnostics {
		if d.Detail.Template.ID == "builtin.template.Exec" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.Exec diagnostic, got: %s", rec)
	}
}

// TestTemplate_Error_SourceMissing verifies missing source file is reported.
func TestTemplate_Error_SourceMissing(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="source-missing",
            src=local("/nonexistent.txt"),
            dest="/out.txt",
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)
	// Note: /nonexistent.txt is not added

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.opDiagnostics {
		if d.Detail.Template.ID == "builtin.template.SourceMissing" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.SourceMissing diagnostic, got: %s", rec)
	}
}

// TestTemplate_Error_EnvKeyNotInValues verifies env key not in values is reported.
func TestTemplate_Error_EnvKeyNotInValues(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="env-key-missing",
            src=inline("{{.port}}"),
            dest="/out.txt",
            data={
                "values": {
                    "port": "8080",
                },
                "env": {
                    "MY_HOST": "host",
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)
	src.Env["MY_HOST"] = "localhost" // Set the env var

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error for env key not in values, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.opDiagnostics {
		if d.Detail.Template.ID == "builtin.template.EnvKeyNotInValues" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.EnvKeyNotInValues diagnostic, got: %s", rec)
	}
}

// TestTemplate_Error_DestDirMissing verifies missing dest directory is reported.
func TestTemplate_Error_DestDirMissing(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="dest-dir-missing",
            src=inline("content"),
            dest="/nonexistent/dir/out.txt",
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error for missing dest directory, got nil")
	}

	// Check for diagnostic with correct ID
	found := false
	for _, d := range rec.opDiagnostics {
		if d.Detail.Template.ID == "builtin.template.DestDirMissing" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin.template.DestDirMissing diagnostic, got: %s", rec)
	}
}

// TestTemplate_ModeChange verifies mode changes are applied.
func TestTemplate_ModeChange(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="mode-change",
            src=inline("content"),
            dest="/out.txt",
            perm="0755",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	// Pre-populate with wrong mode
	tgt.Files["/out.txt"] = []byte("content")
	tgt.Modes["/out.txt"] = fs.FileMode(0o644)
	tgt.Owners["/out.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Mode should be updated
	if tgt.Modes["/out.txt"] != fs.FileMode(0o755) {
		t.Errorf("mode not updated: got %o, want %o", tgt.Modes["/out.txt"], 0o755)
	}
}

// TestTemplate_OwnerChange verifies owner changes are applied.
func TestTemplate_OwnerChange(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="owner-change",
            src=inline("content"),
            dest="/out.txt",
            perm="0644",
            owner="newuser",
            group="newgroup",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	// Pre-populate with wrong owner
	tgt.Files["/out.txt"] = []byte("content")
	tgt.Modes["/out.txt"] = fs.FileMode(0o644)
	tgt.Owners["/out.txt"] = target.Owner{User: "olduser", Group: "oldgroup"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Owner should be updated
	owner := tgt.Owners["/out.txt"]
	if owner.User != "newuser" || owner.Group != "newgroup" {
		t.Errorf("owner not updated: got %+v, want user=newuser group=newgroup", owner)
	}
}

// TestTemplate_MultipleValues verifies multiple values work correctly.
func TestTemplate_MultipleValues(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="multi-values",
            src=inline("{{.host}}:{{.port}} - {{.name}}"),
            dest="/out.txt",
            data={
                "values": {
                    "host": "localhost",
                    "port": 8080,
                    "name": "myapp",
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	expected := "localhost:8080 - myapp"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// TestTemplate_NoData verifies templates work without any data.
func TestTemplate_NoData(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="no-data",
            src=inline("static template"),
            dest="/out.txt",
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	expected := "static template"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// TestTemplate_NestedValues verifies nested data structures work.
func TestTemplate_NestedValues(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="nested",
            src=inline("{{.server.host}}:{{.server.port}}"),
            dest="/out.txt",
            data={
                "values": {
                    "server": {
                        "host": "example.com",
                        "port": 443,
                    },
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	expected := "example.com:443"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// TestTemplate_MultipleEnvOverrides verifies multiple env overrides work.
func TestTemplate_MultipleEnvOverrides(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="multi-env",
            src=inline("{{.host}}:{{.port}}"),
            dest="/out.txt",
            data={
                "values": {
                    "host": "localhost",
                    "port": "8080",
                },
                "env": {
                    "MY_HOST": "host",
                    "MY_PORT": "port",
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)
	src.Env["MY_HOST"] = "prod.example.com"
	src.Env["MY_PORT"] = "443"

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	expected := "prod.example.com:443"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// TestTemplate_PartialEnvOverride verifies some env vars override while others use defaults.
func TestTemplate_PartialEnvOverride(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="partial-env",
            src=inline("{{.host}}:{{.port}}"),
            dest="/out.txt",
            data={
                "values": {
                    "host": "localhost",
                    "port": "8080",
                },
                "env": {
                    "MY_HOST": "host",
                    "MY_PORT": "port",
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/config.star"] = []byte(cfgStr)
	// Only set MY_HOST, not MY_PORT
	src.Env["MY_HOST"] = "prod.example.com"

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// MY_HOST overridden, MY_PORT uses default
	expected := "prod.example.com:8080"
	if string(tgt.Files["/out.txt"]) != expected {
		t.Errorf("unexpected content: got %q, want %q", tgt.Files["/out.txt"], expected)
	}
}

// TestTemplate_WriteFailure verifies write failure is handled.
func TestTemplate_WriteFailure(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="write-fail",
            src=inline("content"),
            dest="/out.txt",
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := newFaultyTarget(innerTgt)

	src.Files["/config.star"] = []byte(cfgStr)

	// Inject write failure
	tgt.injectFault("WriteFile", "/out.txt", fs.ErrPermission)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = mockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	err = e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error for write failure, got nil")
	}
}
