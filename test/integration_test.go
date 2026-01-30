package test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

// TestIntegration_FullFlow tests the complete engine flow from config loading
// through execution using in-memory source and target.
func TestIntegration_FullFlow(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "copy-test"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "testuser"
		group: "testgroup"
	}
]
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("test content")
	src.Files["/config.cue"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
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
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "idempotent-copy"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "owner"
		group: "group"
	}
]
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")
	src.Files["/config.cue"] = []byte(cfgStr)

	// Pre-populate target with matching state
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "owner", Group: "group"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
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
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "copy-1"
		src:   "/src-a.txt"
		dest:  "/dest-a.txt"
		perm:  "0644"
		owner: "user"
		group: "group"
	},
	builtin.copy & {
		desc:  "copy-2"
		src:   "/src-b.txt"
		dest:  "/dest-b.txt"
		perm:  "0600"
		owner: "user"
		group: "group"
	},
]
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src-a.txt"] = []byte("file A")
	src.Files["/src-b.txt"] = []byte("file B")
	src.Files["/config.cue"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
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
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "will-fail"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "user"
		group: "group"
	}
]
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := newFaultyTarget(innerTgt)

	src.Files["/src.txt"] = []byte("content")
	src.Files["/config.cue"] = []byte(cfgStr)

	// Inject write failure
	writeErr := errors.New("disk full")
	tgt.injectFault("WriteFile", "/dest.txt", writeErr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
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
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "source-fail"
		src:   "/missing.txt"
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "user"
		group: "group"
	}
]
`
	innerSrc := source.NewMemSource()
	src := newFaultySource(innerSrc)
	tgt := target.NewMemTarget()

	innerSrc.Files["/config.cue"] = []byte(cfgStr)
	// Note: /missing.txt is not added, so read will fail

	// Inject explicit error
	readErr := errors.New("permission denied")
	src.injectFault("/missing.txt", readErr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestIntegration_PartialFailure verifies that first action failure aborts
// subsequent actions (fail-fast).
func TestIntegration_PartialFailure(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "first-fails"
		src:   "/src-a.txt"
		dest:  "/dest-a.txt"
		perm:  "0644"
		owner: "user"
		group: "group"
	},
	builtin.copy & {
		desc:  "never-runs"
		src:   "/src-b.txt"
		dest:  "/dest-b.txt"
		perm:  "0644"
		owner: "user"
		group: "group"
	},
]
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := newFaultyTarget(innerTgt)

	src.Files["/src-a.txt"] = []byte("A")
	src.Files["/src-b.txt"] = []byte("B")
	src.Files["/config.cue"] = []byte(cfgStr)

	// First write fails
	tgt.injectFault("WriteFile", "/dest-a.txt", errors.New("write failed"))

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Second file should NOT be written (fail-fast)
	if _, exists := innerTgt.Files["/dest-b.txt"]; exists {
		t.Error("second file should not be written due to fail-fast")
	}

	// Should only have one ActionStarted event (second action never started)
	actionStartCount := 0
	for _, ev := range rec.actionEvents {
		if ev.Kind == event.ActionStarted {
			actionStartCount++
		}
	}
	if actionStartCount != 1 {
		t.Errorf("expected 1 ActionStarted event (fail-fast), got %d", actionStartCount)
	}
}

// TestIntegration_ContentChange verifies that content changes are detected
// and applied correctly.
func TestIntegration_ContentChange(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "update-content"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "user"
		group: "group"
	}
]
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new content")
	src.Files["/config.cue"] = []byte(cfgStr)

	// Pre-populate target with OLD content
	tgt.Files["/dest.txt"] = []byte("old content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
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
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "update-mode"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "0755"
		owner: "user"
		group: "group"
	}
]
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")
	src.Files["/config.cue"] = []byte(cfgStr)

	// Pre-populate target with correct content but WRONG mode
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644) // Different from 0755
	tgt.Owners["/dest.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
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
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "update-owner"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "newuser"
		group: "newgroup"
	}
]
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")
	src.Files["/config.cue"] = []byte(cfgStr)

	// Pre-populate target with correct content and mode but WRONG owner
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "olduser", Group: "oldgroup"}

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
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
package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "retry-test"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "user"
		group: "group"
	}
]
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := newFaultyTarget(innerTgt)

	src.Files["/src.txt"] = []byte("content")
	src.Files["/config.cue"] = []byte(cfgStr)

	// First attempt: inject fault
	tgt.injectFault("WriteFile", "/dest.txt", errors.New("temporary failure"))

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg.Target = mockTargetInstance(tgt)

	e, err := engine.New(src, cfg, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e.Apply(ctx)
	if err == nil {
		t.Fatal("first attempt should fail")
	}

	// Clear fault and retry
	tgt.clearFaults()

	rec2 := &recordingDisplayer{}
	em2 := diagnostic.NewEmitter(diagnostic.Policy{}, rec2)
	store2 := spec.NewSourceStore()

	ctx2 := context.Background()
	cfg2, err := engine.LoadConfig(ctx2, em2, "/config.cue", store2, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	cfg2.Target = mockTargetInstance(tgt)

	e2, err := engine.New(src, cfg2, em2)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}

	err = e2.Apply(ctx2)
	if err != nil {
		t.Fatalf("second attempt should succeed: %v", err)
	}

	// Verify file was written
	if string(innerTgt.Files["/dest.txt"]) != "content" {
		t.Error("file should be written after retry")
	}
}
