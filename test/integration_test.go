// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
)

// loadAndResolve is a helper for integration tests that loads config and resolves
// with a mock target.
func loadAndResolve(
	t *testing.T,
	cfgStr string,
	src source.Source,
	tgt target.Target,
	em diagnostic.Emitter,
	store *diagnostic.SourceStore,
) (*engine.Engine, error) {
	t.Helper()

	ctx := context.Background()

	memSrc, ok := src.(*source.MemSource)
	if ok {
		memSrc.Files["/config.scampi"] = []byte(cfgStr)
	}

	cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
	if err != nil {
		return nil, err
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		return nil, err
	}

	resolved.Target = mockTargetInstance(tgt)

	return engine.New(ctx, src, resolved, em)
}

// TestIntegration_FullFlow tests the complete engine flow from config loading
// through execution using in-memory source and target.
func TestIntegration_FullFlow(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="copy-test",
		src=local("/src.txt"),
		dest="/dest.txt",
		perm="0644",
		owner="testuser",
		group="testgroup",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("test content")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Verify file was copied
	data, ok := tgt.Files["/dest.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	if string(data) != "test content" {
		t.Errorf("unexpected content: got %q, want %q", data, "test content")
	}

	// Verify permissions were set
	mode, ok := tgt.Modes["/dest.txt"]
	if !ok {
		t.Fatal("mode not set")
	}
	if mode != fs.FileMode(0o644) {
		t.Errorf("unexpected mode: got %o, want %o", mode, 0o644)
	}

	// Verify ownership was set
	owner, ok := tgt.Owners["/dest.txt"]
	if !ok {
		t.Fatal("owner not set")
	}
	if owner.User != "testuser" || owner.Group != "testgroup" {
		t.Errorf("unexpected owner: got %+v, want user=testuser group=testgroup", owner)
	}
}

// TestIntegration_Idempotency verifies that a second run skips already-satisfied ops.
func TestIntegration_Idempotency(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="idempotent-copy",
		src=local("/src.txt"),
		dest="/dest.txt",
		perm="0644",
		owner="owner",
		group="group",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")

	// Pre-populate target with matching state
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "owner", Group: "group"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Check that ActionFinished has all ops skipped (no changes, no failures)
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

	// All ops should be skipped
	if actionFinished.Summary.Skipped != actionFinished.Summary.Total {
		t.Errorf("expected all ops skipped: got %d/%d skipped",
			actionFinished.Summary.Skipped, actionFinished.Summary.Total)
	}
	if actionFinished.Summary.Changed != 0 {
		t.Errorf("expected no changes, got %d", actionFinished.Summary.Changed)
	}

	// Verify no OpExecuted events (skipped ops don't execute)
	for _, ev := range rec.opEvents {
		if ev.Kind == event.OpExecuted {
			t.Error("unexpected OpExecuted event - skipped ops should not execute")
		}
	}
}

// TestIntegration_MultipleSteps verifies sequential execution of multiple steps.
func TestIntegration_MultipleSteps(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="copy-1",
		src=local("/src-a.txt"),
		dest="/dest-a.txt",
		perm="0644",
		owner="user",
		group="group",
	),
	copy(
		desc="copy-2",
		src=local("/src-b.txt"),
		dest="/dest-b.txt",
		perm="0600",
		owner="user",
		group="group",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src-a.txt"] = []byte("file A")
	src.Files["/src-b.txt"] = []byte("file B")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Verify both files copied
	if string(tgt.Files["/dest-a.txt"]) != "file A" {
		t.Errorf("file A: got %q, want %q", tgt.Files["/dest-a.txt"], "file A")
	}
	if string(tgt.Files["/dest-b.txt"]) != "file B" {
		t.Errorf("file B: got %q, want %q", tgt.Files["/dest-b.txt"], "file B")
	}

	// Verify different permissions
	if tgt.Modes["/dest-a.txt"] != fs.FileMode(0o644) {
		t.Errorf("file A mode: got %o, want %o", tgt.Modes["/dest-a.txt"], 0o644)
	}
	if tgt.Modes["/dest-b.txt"] != fs.FileMode(0o600) {
		t.Errorf("file B mode: got %o, want %o", tgt.Modes["/dest-b.txt"], 0o600)
	}

	// Verify two ActionFinished events
	actionCount := 0
	for _, ev := range rec.actionEvents {
		if ev.Kind == event.ActionFinished {
			actionCount++
		}
	}
	if actionCount != 2 {
		t.Errorf("expected 2 ActionFinished events, got %d", actionCount)
	}
}

// TestIntegration_ErrorInjection_WriteFailure verifies engine behavior when
// target write fails.
func TestIntegration_ErrorInjection_WriteFailure(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="will-fail",
		src=local("/src.txt"),
		dest="/dest.txt",
		perm="0644",
		owner="user",
		group="group",
	),
])
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := newFaultyTarget(innerTgt)

	src.Files["/src.txt"] = []byte("content")

	// Inject write failure
	writeErr := errors.New("disk full")
	tgt.injectFault("WriteFile", "/dest.txt", writeErr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	// Should fail with AbortError
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

// TestIntegration_ErrorInjection_SourceReadFailure verifies engine behavior
// when source read fails during check.
func TestIntegration_ErrorInjection_SourceReadFailure(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="source-fail",
		src=local("/missing.txt"),
		dest="/dest.txt",
		perm="0644",
		owner="user",
		group="group",
	),
])
`
	innerSrc := source.NewMemSource()
	src := newFaultySource(innerSrc)
	tgt := target.NewMemTarget()

	innerSrc.Files["/config.scampi"] = []byte(cfgStr)
	// Note: /missing.txt is not added, so read will fail

	// Inject explicit error
	readErr := errors.New("permission denied")
	src.injectFault("/missing.txt", readErr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
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
		t.Fatal("expected error, got nil")
	}
}

// TestIntegration_PartialFailure verifies that first action failure aborts
// subsequent actions (fail-fast).
func TestIntegration_PartialFailure(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="first-fails",
		src=local("/src-a.txt"),
		dest="/dest-a.txt",
		perm="0644",
		owner="user",
		group="group",
	),
	copy(
		desc="never-runs",
		src=local("/src-b.txt"),
		dest="/dest-b.txt",
		perm="0644",
		owner="user",
		group="group",
	),
])
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := newFaultyTarget(innerTgt)

	src.Files["/src-a.txt"] = []byte("A")
	src.Files["/src-b.txt"] = []byte("B")

	// First write fails
	tgt.injectFault("WriteFile", "/dest-a.txt", errors.New("write failed"))

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// With parallel execution, independent actions run concurrently.
	// The second copy (independent paths) WILL complete successfully
	// even though the first copy failed.
	if _, exists := innerTgt.Files["/dest-b.txt"]; !exists {
		t.Error("second file should be written (independent action)")
	}

	// Both actions start in parallel (they have independent paths)
	actionStartCount := 0
	for _, ev := range rec.actionEvents {
		if ev.Kind == event.ActionStarted {
			actionStartCount++
		}
	}
	if actionStartCount != 2 {
		t.Errorf("expected 2 ActionStarted events (parallel execution), got %d", actionStartCount)
	}
}

// TestIntegration_ContentChange verifies that content changes are detected
// and applied correctly.
func TestIntegration_ContentChange(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="update-content",
		src=local("/src.txt"),
		dest="/dest.txt",
		perm="0644",
		owner="user",
		group="group",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new content")

	// Pre-populate target with OLD content
	tgt.Files["/dest.txt"] = []byte("old content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Content should be updated
	if string(tgt.Files["/dest.txt"]) != "new content" {
		t.Errorf("content not updated: got %q, want %q",
			tgt.Files["/dest.txt"], "new content")
	}

	// Check ActionFinished for changes
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

	// Should have changes (copy op executed)
	if actionFinished.Summary.Changed == 0 {
		t.Error("expected changes due to content update")
	}
}

// TestIntegration_ModeChange verifies that permission changes are detected
// and applied correctly.
func TestIntegration_ModeChange(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="update-mode",
		src=local("/src.txt"),
		dest="/dest.txt",
		perm="0755",
		owner="user",
		group="group",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")

	// Pre-populate target with correct content but WRONG mode
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644) // Different from 0755
	tgt.Owners["/dest.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Mode should be updated
	if tgt.Modes["/dest.txt"] != fs.FileMode(0o755) {
		t.Errorf("mode not updated: got %o, want %o",
			tgt.Modes["/dest.txt"], 0o755)
	}
}

// TestIntegration_OwnerChange verifies that ownership changes are detected
// and applied correctly.
func TestIntegration_OwnerChange(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="update-owner",
		src=local("/src.txt"),
		dest="/dest.txt",
		perm="0644",
		owner="newuser",
		group="newgroup",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")

	// Pre-populate target with correct content and mode but WRONG owner
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "olduser", Group: "oldgroup"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Owner should be updated
	owner := tgt.Owners["/dest.txt"]
	if owner.User != "newuser" || owner.Group != "newgroup" {
		t.Errorf("owner not updated: got %+v, want user=newuser group=newgroup", owner)
	}
}

// TestIntegration_FaultyClearAndRetry verifies that clearing faults allows retry.
func TestIntegration_FaultyClearAndRetry(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	copy(
		desc="retry-test",
		src=local("/src.txt"),
		dest="/dest.txt",
		perm="0644",
		owner="user",
		group="group",
	),
])
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := newFaultyTarget(innerTgt)

	src.Files["/src.txt"] = []byte("content")

	// First attempt: inject fault
	tgt.injectFault("WriteFile", "/dest.txt", errors.New("temporary failure"))

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("first attempt should fail")
	}

	// Clear fault and retry
	tgt.clearFaults()

	rec2 := &recordingDisplayer{}
	em2 := diagnostic.NewEmitter(diagnostic.Policy{}, rec2)
	store2 := diagnostic.NewSourceStore()

	e2, err := loadAndResolve(t, cfgStr, src, tgt, em2, store2)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e2.Close()

	err = e2.Apply(context.Background())
	if err != nil {
		t.Fatalf("second attempt should succeed: %v", err)
	}

	// Verify file was written
	if string(innerTgt.Files["/dest.txt"]) != "content" {
		t.Error("file should be written after retry")
	}
}

// Hook tests
// -----------------------------------------------------------------------------

// TestHook_Triggered verifies that a hook fires when its notifying step changes.
func TestHook_Triggered(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["restart-nginx"],
		),
	],
	hooks={
		"restart-nginx": service(
			name="nginx",
			state="restarted",
		),
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["nginx"] != 1 {
		t.Errorf("expected 1 restart, got %d", tgt.Restarts["nginx"])
	}
}

// TestHook_OnChangeSingleString verifies that on_change accepts a bare string.
func TestHook_OnChangeSingleString(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change="restart-nginx",
		),
	],
	hooks={
		"restart-nginx": service(
			name="nginx",
			state="restarted",
		),
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["nginx"] != 1 {
		t.Errorf("expected 1 restart, got %d", tgt.Restarts["nginx"])
	}
}

// TestHook_NotTriggered verifies that a hook does not fire when nothing changed.
func TestHook_NotTriggered(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["restart-nginx"],
		),
	],
	hooks={
		"restart-nginx": service(
			name="nginx",
			state="restarted",
		),
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")
	// Pre-populate with matching state
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "user", Group: "group"}
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["nginx"] != 0 {
		t.Errorf("expected 0 restarts, got %d", tgt.Restarts["nginx"])
	}
}

// TestHook_MultipleNotifiers verifies a hook fires once even if notified
// by multiple steps.
func TestHook_MultipleNotifiers(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-a",
			src=local("/src-a.txt"),
			dest="/dest-a.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["restart-nginx"],
		),
		copy(
			desc="config-b",
			src=local("/src-b.txt"),
			dest="/dest-b.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["restart-nginx"],
		),
	],
	hooks={
		"restart-nginx": service(
			name="nginx",
			state="restarted",
		),
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src-a.txt"] = []byte("config A")
	src.Files["/src-b.txt"] = []byte("config B")
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["nginx"] != 1 {
		t.Errorf("expected exactly 1 restart, got %d", tgt.Restarts["nginx"])
	}
}

// TestHook_Chaining verifies that hooks can trigger other hooks.
func TestHook_Chaining(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["restart-app"],
		),
	],
	hooks={
		"restart-app": service(
			name="app",
			state="restarted",
			on_change=["restart-proxy"],
		),
		"restart-proxy": service(
			name="proxy",
			state="restarted",
		),
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.Services["app"] = true
	tgt.EnabledServices["app"] = true
	tgt.Services["proxy"] = true
	tgt.EnabledServices["proxy"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["app"] != 1 {
		t.Errorf("expected 1 app restart, got %d", tgt.Restarts["app"])
	}
	if tgt.Restarts["proxy"] != 1 {
		t.Errorf("expected 1 proxy restart, got %d", tgt.Restarts["proxy"])
	}
}

// TestHook_CheckMode verifies that hooks report WouldChange when the
// upstream step would change.
func TestHook_CheckMode(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["restart-nginx"],
		),
	],
	hooks={
		"restart-nginx": service(
			name="nginx",
			state="restarted",
		),
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Check(context.Background()); err != nil {
		t.Fatalf("Check failed: %v\n%s", err, rec)
	}

	// In check mode, nothing should actually restart
	if tgt.Restarts["nginx"] != 0 {
		t.Errorf("check mode should not restart, got %d", tgt.Restarts["nginx"])
	}

	// But the hook action should report WouldChange
	hookTriggered := false
	for _, ev := range rec.actionEvents {
		if ev.Kind == event.HookTriggered {
			hookTriggered = true
			break
		}
	}
	if !hookTriggered {
		t.Error("expected HookTriggered event in check mode")
	}
}

// TestHook_UnknownRef verifies that referencing a nonexistent hook ID
// produces a plan error.
func TestHook_UnknownRef(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["nonexistent-hook"],
		),
	],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	src.Files["/src.txt"] = []byte("content")

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error for unknown hook reference")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

// TestHook_CycleDetection verifies that a hook chain forming a cycle
// produces a plan error.
func TestHook_CycleDetection(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["hook-a"],
		),
	],
	hooks={
		"hook-a": service(
			name="svc-a",
			state="restarted",
			on_change=["hook-b"],
		),
		"hook-b": service(
			name="svc-b",
			state="restarted",
			on_change=["hook-a"],
		),
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	src.Files["/src.txt"] = []byte("content")
	tgt.Services["svc-a"] = true
	tgt.EnabledServices["svc-a"] = true
	tgt.Services["svc-b"] = true
	tgt.EnabledServices["svc-b"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error for hook cycle")
	}

	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Errorf("expected AbortError, got %T: %v", err, err)
	}
}

// TestHook_RunStepAsHook verifies that non-service steps can be hooks.
func TestHook_RunStepAsHook(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["run-reload"],
		),
	],
	hooks={
		"run-reload": run(
			apply="nginx -s reload",
			always=True,
		),
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.CommandFunc = func(_ string) (target.CommandResult, error) {
		return target.CommandResult{ExitCode: 0}, nil
	}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Verify the run hook command was executed
	found := false
	for _, c := range tgt.Commands {
		if c.Cmd == "nginx -s reload" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected run hook command to be executed")
	}
}

// TestHook_MultiStep verifies that a hook with multiple steps executes all
// steps sequentially.
func TestHook_MultiStep(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["deploy-app"],
		),
	],
	hooks={
		"deploy-app": [
			copy(
				desc="app-conf",
				src=local("/app.conf"),
				dest="/app.conf.deployed",
				perm="0644",
				owner="user",
				group="group",
			),
			service(
				name="app",
				state="restarted",
			),
		],
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	src.Files["/app.conf"] = []byte("app config")
	tgt.Services["app"] = true
	tgt.EnabledServices["app"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if string(tgt.Files["/app.conf.deployed"]) != "app config" {
		t.Errorf("app.conf not copied: got %q", tgt.Files["/app.conf.deployed"])
	}
	if tgt.Restarts["app"] != 1 {
		t.Errorf("expected 1 app restart, got %d", tgt.Restarts["app"])
	}
}

// TestHook_MultiStepChaining verifies that on_change in a multi-step hook
// fires downstream hooks when any step changes.
func TestHook_MultiStepChaining(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
	name="test",
	targets=["local"],
	steps=[
		copy(
			desc="config-file",
			src=local("/src.txt"),
			dest="/dest.txt",
			perm="0644",
			owner="user",
			group="group",
			on_change=["deploy-app"],
		),
	],
	hooks={
		"deploy-app": [
			copy(
				desc="app-conf",
				src=local("/app.conf"),
				dest="/app.conf.deployed",
				perm="0644",
				owner="user",
				group="group",
			),
			service(
				name="app",
				state="restarted",
				on_change=["reload-proxy"],
			),
		],
		"reload-proxy": service(
			name="proxy",
			state="restarted",
		),
	},
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	src.Files["/app.conf"] = []byte("app config")
	tgt.Services["app"] = true
	tgt.EnabledServices["app"] = true
	tgt.Services["proxy"] = true
	tgt.EnabledServices["proxy"] = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["app"] != 1 {
		t.Errorf("expected 1 app restart, got %d", tgt.Restarts["app"])
	}
	if tgt.Restarts["proxy"] != 1 {
		t.Errorf("expected 1 proxy restart, got %d", tgt.Restarts["proxy"])
	}
}

// TestIntegration_ReloadFallbackToRestart verifies that state="reloaded"
// falls back to restart when the backend does not support reload.
func TestIntegration_ReloadFallbackToRestart(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(name="test", targets=["local"], steps=[
	service(
		desc="reload-or-restart nginx",
		name="nginx",
		state="reloaded",
	),
])
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true
	tgt.ReloadUnsupported = true

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["nginx"] != 1 {
		t.Errorf("expected 1 restart, got %d", tgt.Restarts["nginx"])
	}
	if tgt.Reloads["nginx"] != 0 {
		t.Errorf("expected 0 reloads, got %d", tgt.Reloads["nginx"])
	}
}
