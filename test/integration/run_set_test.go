// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"strings"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/test/harness"
)

// memCmdResponder builds a CommandFunc that returns canned responses
// keyed by exact command string. Unknown commands return exit 127.
type memCmdResponder struct {
	responses map[string]target.CommandResult
	calls     []string
}

func newMemCmdResponder() *memCmdResponder {
	return &memCmdResponder{responses: map[string]target.CommandResult{}}
}

func (r *memCmdResponder) on(cmd string, res target.CommandResult) *memCmdResponder {
	r.responses[cmd] = res
	return r
}

func (r *memCmdResponder) fn() func(cmd string) (target.CommandResult, error) {
	return func(cmd string) (target.CommandResult, error) {
		r.calls = append(r.calls, cmd)
		if res, ok := r.responses[cmd]; ok {
			return res, nil
		}
		return target.CommandResult{ExitCode: 127, Stderr: "command not found: " + cmd}, nil
	}
}

func TestRunSet_AddsMissing_BatchCSV(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.run_set {
    desc    = "samba admins"
    list    = "list-admins"
    add     = "add-admins {{ items_csv }}"
    remove  = "remove-admins {{ items_csv }}"
    desired = ["alice", "bob", "carol"]
  }
}
`
	resp := newMemCmdResponder().
		on("list-admins", target.CommandResult{ExitCode: 0, Stdout: "alice\n"}).
		on("add-admins bob,carol", target.CommandResult{ExitCode: 0})

	tgt := target.NewMemTarget()
	tgt.CommandFunc = resp.fn()

	rec := runApply(t, cfgStr, tgt)
	if rec == nil {
		t.Fatalf("apply failed")
	}

	want := []string{"list-admins", "add-admins bob,carol"}
	if !equalStrings(resp.calls, want) {
		t.Fatalf("commands = %v\n  want %v", resp.calls, want)
	}
}

func TestRunSet_RemovesOrphans_BatchSpace(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.run_set {
    desc    = "iface allowlist"
    list    = "list-iface"
    add     = "add-iface {{ items }}"
    remove  = "remove-iface {{ items }}"
    desired = ["eth0"]
  }
}
`
	resp := newMemCmdResponder().
		on("list-iface", target.CommandResult{ExitCode: 0, Stdout: "eth0\nstale1\nstale2\n"}).
		on("remove-iface stale1 stale2", target.CommandResult{ExitCode: 0})

	tgt := target.NewMemTarget()
	tgt.CommandFunc = resp.fn()

	rec := runApply(t, cfgStr, tgt)
	if rec == nil {
		t.Fatalf("apply failed")
	}

	want := []string{"list-iface", "remove-iface stale1 stale2"}
	if !equalStrings(resp.calls, want) {
		t.Fatalf("commands = %v\n  want %v", resp.calls, want)
	}
}

func TestRunSet_BothSides(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.run_set {
    desc    = "group sync"
    list    = "list"
    add     = "add {{ items_csv }}"
    remove  = "remove {{ items_csv }}"
    desired = ["alice", "bob"]
  }
}
`
	resp := newMemCmdResponder().
		on("list", target.CommandResult{ExitCode: 0, Stdout: "alice\nzed\n"}).
		on("remove zed", target.CommandResult{ExitCode: 0}).
		on("add bob", target.CommandResult{ExitCode: 0})

	tgt := target.NewMemTarget()
	tgt.CommandFunc = resp.fn()

	rec := runApply(t, cfgStr, tgt)
	if rec == nil {
		t.Fatalf("apply failed")
	}

	// Order matters: remove BEFORE add, both with the resolved batches.
	wantSuffix := []string{"remove zed", "add bob"}
	if !endsWith(resp.calls, wantSuffix) {
		t.Fatalf("commands = %v\n  want suffix %v", resp.calls, wantSuffix)
	}
}

func TestRunSet_NoopWhenConverged(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.run_set {
    list    = "list"
    add     = "add {{ items_csv }}"
    remove  = "remove {{ items_csv }}"
    desired = ["alice", "bob"]
  }
}
`
	resp := newMemCmdResponder().
		on("list", target.CommandResult{ExitCode: 0, Stdout: "alice\nbob\n"})

	tgt := target.NewMemTarget()
	tgt.CommandFunc = resp.fn()

	rec := runApply(t, cfgStr, tgt)
	if rec == nil {
		t.Fatalf("apply failed")
	}

	// Only the check-time list call should have happened — no second list,
	// no add, no remove, since live == desired.
	if len(resp.calls) != 1 {
		t.Fatalf("expected 1 list call, got %d: %v", len(resp.calls), resp.calls)
	}
}

func TestRunSet_PerItemTemplate(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.run_set {
    list    = "list"
    add     = "addone {{ item }}"
    desired = ["a", "b", "c"]
  }
}
`
	resp := newMemCmdResponder().
		on("list", target.CommandResult{ExitCode: 0, Stdout: ""}).
		on("addone a", target.CommandResult{ExitCode: 0}).
		on("addone b", target.CommandResult{ExitCode: 0}).
		on("addone c", target.CommandResult{ExitCode: 0})

	tgt := target.NewMemTarget()
	tgt.CommandFunc = resp.fn()

	rec := runApply(t, cfgStr, tgt)
	if rec == nil {
		t.Fatalf("apply failed")
	}

	wantSuffix := []string{"addone a", "addone b", "addone c"}
	if !endsWith(resp.calls, wantSuffix) {
		t.Fatalf("commands = %v\n  want suffix %v", resp.calls, wantSuffix)
	}
}

func TestRunSet_InitBootstraps(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.run_set {
    list    = "list"
    init    = "init-container"
    add     = "add {{ items_csv }}"
    desired = ["alice"]
  }
}
`
	// First list call fails (container missing); init succeeds; second
	// list call returns empty; add fires.
	listCalls := 0
	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
		switch cmd {
		case "list":
			listCalls++
			if listCalls == 1 {
				return target.CommandResult{ExitCode: 2, Stderr: "no such group"}, nil
			}
			return target.CommandResult{ExitCode: 0, Stdout: ""}, nil
		case "init-container":
			return target.CommandResult{ExitCode: 0}, nil
		case "add alice":
			return target.CommandResult{ExitCode: 0}, nil
		}
		return target.CommandResult{ExitCode: 127, Stderr: "unknown: " + cmd}, nil
	}

	rec := runApply(t, cfgStr, tgt)
	if rec == nil {
		t.Fatalf("apply failed")
	}

	calls := tgt.CommandStrings()
	// list (fail) → init → list (succeed) → add alice.
	wantSubseq := []string{"list", "init-container", "list", "add alice"}
	if !containsSubsequence(calls, wantSubseq) {
		t.Fatalf("commands = %v\n  want subsequence %v", calls, wantSubseq)
	}
}

func TestRunSet_AddOnly_LeavesOrphans(t *testing.T) {
	// User declared `add` but no `remove` — orphans must NOT trigger
	// drift. Live set has an item not in desired; the step must still
	// be considered satisfied (one-way reconciliation).
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.run_set {
    list    = "list"
    add     = "add {{ items_csv }}"
    desired = ["alice"]
  }
}
`
	resp := newMemCmdResponder().
		on("list", target.CommandResult{ExitCode: 0, Stdout: "alice\nstale\n"})

	tgt := target.NewMemTarget()
	tgt.CommandFunc = resp.fn()

	rec := runApply(t, cfgStr, tgt)
	if rec == nil {
		t.Fatalf("apply failed")
	}

	// Only the check-time list call: live ⊇ desired, add disabled means
	// no drift to act on.
	if len(resp.calls) != 1 {
		t.Fatalf("expected only 1 list call, got %d: %v", len(resp.calls), resp.calls)
	}
}

func TestRunSet_EnvPrefixApplied(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.run_set {
    list    = "list"
    add     = "add {{ items_csv }}"
    env     = {"FOO": "bar"}
    desired = ["x"]
  }
}
`
	resp := newMemCmdResponder().
		on("FOO='bar' list", target.CommandResult{ExitCode: 0, Stdout: ""}).
		on("FOO='bar' add x", target.CommandResult{ExitCode: 0})

	tgt := target.NewMemTarget()
	tgt.CommandFunc = resp.fn()

	rec := runApply(t, cfgStr, tgt)
	if rec == nil {
		t.Fatalf("apply failed: env not applied to commands? calls=%v", resp.calls)
	}

	for _, c := range resp.calls {
		if !strings.HasPrefix(c, "FOO='bar' ") {
			t.Errorf("command missing env prefix: %q", c)
		}
	}
}

// runApply loads a config, runs Apply against tgt, fails the test on
// load/apply error, returns the recording displayer for further
// assertions. A nil return means apply failed.
func runApply(t *testing.T, cfgStr string, tgt target.Target) *harness.RecordingDisplayer {
	t.Helper()
	src := source.NewMemSource()
	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v\n%s", err, rec)
		return nil
	}
	defer e.Close()

	if err := e.Apply(context.Background()); err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
		return nil
	}
	return rec
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func endsWith(seq, suffix []string) bool {
	if len(seq) < len(suffix) {
		return false
	}
	off := len(seq) - len(suffix)
	for i, s := range suffix {
		if seq[off+i] != s {
			return false
		}
	}
	return true
}

func containsSubsequence(haystack, needle []string) bool {
	if len(needle) == 0 {
		return true
	}
	hi := 0
	for _, ns := range needle {
		for ; hi < len(haystack); hi++ {
			if haystack[hi] == ns {
				hi++
				goto next
			}
		}
		return false
	next:
	}
	return true
}
