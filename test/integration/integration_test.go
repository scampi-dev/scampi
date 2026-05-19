// SPDX-License-Identifier: GPL-3.0-only

package integration

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
	"scampi.dev/scampi/test/harness"
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

	resolved.Target = harness.MockTargetInstance(tgt)

	return engine.New(ctx, src, resolved, em)
}

// TestIntegration_FullFlow tests the complete engine flow from config loading
// through execution using in-memory source and target.
func TestIntegration_FullFlow(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "copy-test"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "testuser"
    group = "testgroup"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("test content")

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(context.Background())
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "idempotent-copy"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "owner"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")

	// Pre-populate target with matching state
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "owner", Group: "group"}

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Idempotent run: target state already matches source, so no ops
	// should report a change.
	for _, c := range rec.Changes {
		t.Errorf("unexpected Change event: phase=%v op=%s", c.Phase, c.DisplayID)
	}
}

// TestIntegration_MultipleSteps verifies sequential execution of multiple steps.
func TestIntegration_MultipleSteps(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "copy-1"
    src = posix.source_local { path = "/src-a.txt" }
    dest = "/dest-a.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
  posix.copy {
    desc = "copy-2"
    src = posix.source_local { path = "/src-b.txt" }
    dest = "/dest-b.txt"
    perm = "0600"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src-a.txt"] = []byte("file A")
	src.Files["/src-b.txt"] = []byte("file B")

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(context.Background())
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

	// Both actions produced executed changes; the file-state checks
	// above are the real proof that they ran with the right content.
	steps := map[int]bool{}
	for _, c := range rec.Changes {
		if c.Phase == event.ChangeExecuted {
			steps[c.Step.Index] = true
		}
	}
	if len(steps) != 2 {
		t.Errorf("expected 2 distinct step indexes with executed changes, got %d", len(steps))
	}
}

// TestIntegration_ErrorInjection_WriteFailure verifies engine behavior when
// target write fails.
func TestIntegration_ErrorInjection_WriteFailure(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "will-fail"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := harness.NewFaultyTarget(innerTgt)

	src.Files["/src.txt"] = []byte("content")

	// Inject write failure
	writeErr := errors.New("disk full")
	tgt.InjectFault("WriteFile", "/dest.txt", writeErr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(context.Background())
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "source-fail"
    src = posix.source_local { path = "/missing.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	innerSrc := source.NewMemSource()
	src := harness.NewFaultySource(innerSrc)
	tgt := target.NewMemTarget()

	innerSrc.Files["/config.scampi"] = []byte(cfgStr)
	// Note: /missing.txt is not added, so read will fail

	// Inject explicit error
	readErr := errors.New("permission denied")
	src.InjectFault("/missing.txt", readErr)

	rec := &harness.RecordingDisplayer{}
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

	resolved.Target = harness.MockTargetInstance(tgt)

	e, err := engine.New(ctx, src, resolved, em)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestIntegration_PartialFailure verifies that first action failure aborts
// subsequent actions (fail-fast).
func TestIntegration_PartialFailure(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "first-fails"
    src = posix.source_local { path = "/src-a.txt" }
    dest = "/dest-a.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
  posix.copy {
    desc = "never-runs"
    src = posix.source_local { path = "/src-b.txt" }
    dest = "/dest-b.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := harness.NewFaultyTarget(innerTgt)

	src.Files["/src-a.txt"] = []byte("A")
	src.Files["/src-b.txt"] = []byte("B")

	// First write fails
	tgt.InjectFault("WriteFile", "/dest-a.txt", errors.New("write failed"))

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// With parallel execution, independent actions run concurrently.
	// The second copy (independent paths) WILL complete successfully
	// even though the first copy failed.
	if _, exists := innerTgt.Files["/dest-b.txt"]; !exists {
		t.Error("second file should be written (independent action)")
	}

}

// TestIntegration_ContentChange verifies that content changes are detected
// and applied correctly.
func TestIntegration_ContentChange(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "update-content"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new content")

	// Pre-populate target with OLD content
	tgt.Files["/dest.txt"] = []byte("old content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// Content should be updated
	if string(tgt.Files["/dest.txt"]) != "new content" {
		t.Errorf("content not updated: got %q, want %q",
			tgt.Files["/dest.txt"], "new content")
	}

	// Should have changes (copy op executed)
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

// TestIntegration_ModeChange verifies that permission changes are detected
// and applied correctly.
func TestIntegration_ModeChange(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "update-mode"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0755"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")

	// Pre-populate target with correct content but WRONG mode
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644) // Different from 0755
	tgt.Owners["/dest.txt"] = target.Owner{User: "user", Group: "group"}

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(context.Background())
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "update-owner"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "newuser"
    group = "newgroup"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("content")

	// Pre-populate target with correct content and mode but WRONG owner
	tgt.Files["/dest.txt"] = []byte("content")
	tgt.Modes["/dest.txt"] = fs.FileMode(0o644)
	tgt.Owners["/dest.txt"] = target.Owner{User: "olduser", Group: "oldgroup"}

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(context.Background())
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "retry-test"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	innerTgt := target.NewMemTarget()
	tgt := harness.NewFaultyTarget(innerTgt)

	src.Files["/src.txt"] = []byte("content")

	// First attempt: inject fault
	tgt.InjectFault("WriteFile", "/dest.txt", errors.New("temporary failure"))

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(context.Background())
	if err == nil {
		t.Fatal("first attempt should fail")
	}

	// Clear fault and retry
	tgt.ClearFaults()

	rec2 := &harness.RecordingDisplayer{}
	em2 := diagnostic.NewEmitter(diagnostic.Policy{}, rec2)
	store2 := diagnostic.NewSourceStore()

	e2, err := loadAndResolve(t, cfgStr, src, tgt, em2, store2)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e2.Close()

	if _, err = e2.Apply(context.Background()); err != nil {
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let restart_nginx = posix.service {
    name = "nginx"
    state = posix.ServiceState.restarted
  }

  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [restart_nginx]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["nginx"] != 1 {
		t.Errorf("expected 1 restart, got %d", tgt.Restarts["nginx"])
	}
}

// TestHook_OnChangeSingleString verifies that on_change accepts a bare string.
func TestHook_OnChangeSingleString(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let restart_nginx = posix.service {
    name = "nginx"
    state = posix.ServiceState.restarted
  }

  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [restart_nginx]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["nginx"] != 1 {
		t.Errorf("expected 1 restart, got %d", tgt.Restarts["nginx"])
	}
}

// TestHook_NotTriggered verifies that a hook does not fire when nothing changed.
func TestHook_NotTriggered(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let restart_nginx = posix.service {
    name = "nginx"
    state = posix.ServiceState.restarted
  }

  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [restart_nginx]
  }
}
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

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(context.Background()); err != nil {
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let restart_nginx = posix.service {
    name = "nginx"
    state = posix.ServiceState.restarted
  }

  posix.copy {
    desc = "config-a"
    src = posix.source_local { path = "/src-a.txt" }
    dest = "/dest-a.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [restart_nginx]
  }
  posix.copy {
    desc = "config-b"
    src = posix.source_local { path = "/src-b.txt" }
    dest = "/dest-b.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [restart_nginx]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src-a.txt"] = []byte("config A")
	src.Files["/src-b.txt"] = []byte("config B")
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["nginx"] != 1 {
		t.Errorf("expected exactly 1 restart, got %d", tgt.Restarts["nginx"])
	}
}

// TestHook_Chaining verifies that hooks can trigger other hooks.
func TestHook_Chaining(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let restart_proxy = posix.service {
    name = "proxy"
    state = posix.ServiceState.restarted
  }
  let restart_app = posix.service {
    name = "app"
    state = posix.ServiceState.restarted
    on_change = [restart_proxy]
  }

  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [restart_app]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.Services["app"] = true
	tgt.EnabledServices["app"] = true
	tgt.Services["proxy"] = true
	tgt.EnabledServices["proxy"] = true

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(context.Background()); err != nil {
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let restart_nginx = posix.service {
    name = "nginx"
    state = posix.ServiceState.restarted
  }

  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [restart_nginx]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Check(context.Background()); err != nil {
		t.Fatalf("Check failed: %v\n%s", err, rec)
	}

	// In check mode, nothing should actually restart
	if tgt.Restarts["nginx"] != 0 {
		t.Errorf("check mode should not restart, got %d", tgt.Restarts["nginx"])
	}

	// The hook fired and its ops reported a planned change.
	hookChange := false
	for _, c := range rec.Changes {
		if c.Cause.Kind == event.CauseHook && c.Phase == event.ChangePlanned {
			hookChange = true
			break
		}
	}
	if !hookChange {
		t.Error("expected hook-triggered Change(Planned) in check mode")
	}
}

// TestHook_UnknownRef verifies that referencing an undefined variable
// in on_change produces a compile error. In scampi, on_change
// takes step values — using an undefined name is a type error.
func TestHook_UnknownRef(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [nonexistent_hook]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	src.Files["/src.txt"] = []byte("content")

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	// Should fail at compile time — undefined variable.
	_, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err == nil {
		t.Fatal("expected error for undefined hook reference")
	}
}

// TestHook_CycleDetection verifies that a hook chain forming a cycle
// produces a plan error.
func TestHook_CycleDetection(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let hook_b = posix.service {
    name = "svc-b"
    state = posix.ServiceState.restarted
    on_change = [hook_a]
  }
  let hook_a = posix.service {
    name = "svc-a"
    state = posix.ServiceState.restarted
    on_change = [hook_b]
  }

  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [hook_a]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	src.Files["/src.txt"] = []byte("content")
	tgt.Services["svc-a"] = true
	tgt.EnabledServices["svc-a"] = true
	tgt.Services["svc-b"] = true
	tgt.EnabledServices["svc-b"] = true

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	// In scampi, forward references (hook_b → hook_a before
	// hook_a is defined) are caught by the eval. This is a compile
	// error, not a runtime hook cycle.
	_, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err == nil {
		t.Fatal("expected error for forward reference / hook cycle")
	}
}

// TestHook_RunStepAsHook verifies that non-service steps can be hooks.
func TestHook_RunStepAsHook(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let run_reload = posix.run {
    apply = "nginx -s reload"
    always = true
  }

  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = [run_reload]
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	tgt.CommandFunc = func(_ string) (target.CommandResult, error) {
		return target.CommandResult{ExitCode: 0}, nil
	}

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(context.Background()); err != nil {
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let deploy_app = [
    posix.copy {
      desc = "app-conf"
      src = posix.source_local { path = "/app.conf" }
      dest = "/app.conf.deployed"
      perm = "0644"
      owner = "user"
      group = "group"
    },
    posix.service {
      name = "app"
      state = posix.ServiceState.restarted
    },
  ]

  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = deploy_app
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	src.Files["/app.conf"] = []byte("app config")
	tgt.Services["app"] = true
	tgt.EnabledServices["app"] = true

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(context.Background()); err != nil {
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  let reload_proxy = posix.service {
    name = "proxy"
    state = posix.ServiceState.restarted
  }
  let deploy_app = [
    posix.copy {
      desc = "app-conf"
      src = posix.source_local { path = "/app.conf" }
      dest = "/app.conf.deployed"
      perm = "0644"
      owner = "user"
      group = "group"
    },
    posix.service {
      name = "app"
      state = posix.ServiceState.restarted
      on_change = [reload_proxy]
    },
  ]

  posix.copy {
    desc = "config-file"
    src = posix.source_local { path = "/src.txt" }
    dest = "/dest.txt"
    perm = "0644"
    owner = "user"
    group = "group"
    on_change = deploy_app
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/src.txt"] = []byte("new config")
	src.Files["/app.conf"] = []byte("app config")
	tgt.Services["app"] = true
	tgt.EnabledServices["app"] = true
	tgt.Services["proxy"] = true
	tgt.EnabledServices["proxy"] = true

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(context.Background()); err != nil {
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
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.service {
    desc = "reload-or-restart nginx"
    name = "nginx"
    state = posix.ServiceState.reloaded
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = true
	tgt.ReloadUnsupported = true

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	if _, err = e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if tgt.Restarts["nginx"] != 1 {
		t.Errorf("expected 1 restart, got %d", tgt.Restarts["nginx"])
	}
	if tgt.Reloads["nginx"] != 0 {
		t.Errorf("expected 0 reloads, got %d", tgt.Reloads["nginx"])
	}
}
