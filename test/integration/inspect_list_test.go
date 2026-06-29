// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/test/harness"
)

func writeInspectCfg(t *testing.T, cfg string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.scampi")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	return path
}

// TestInspectList_SingleDeploy verifies the return value of
// engine.InspectList for a single deploy with an inspectable step.
// posix.symlink registers Inspect fields for target/link/owner.
func TestInspectList_SingleDeploy(t *testing.T) {
	cfg := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "solo", targets = [host]) {
  posix.symlink {
    desc   = "link config"
    target = "/etc/app.conf"
    link   = "/etc/current.conf"
  }
}
`
	path := writeInspectCfg(t, cfg)
	store := diagnostic.NewSourceStore()
	em := diagnostic.NewEmitter(diagnostic.Policy{}, &harness.RecordingDisplayer{})

	details, err := engine.InspectList(diagnostic.NewCtx(t.Context(), em), path, store, spec.ResolveOptions{})
	if err != nil {
		t.Fatalf("engine.InspectList: %v", err)
	}

	if got := len(details); got != 1 {
		t.Fatalf("details: got %d, want 1", got)
	}
	d := details[0]
	if d.DeployName != "solo" {
		t.Errorf("DeployName: got %q, want %q", d.DeployName, "solo")
	}
	if d.TargetName != "local" {
		t.Errorf("TargetName: got %q, want %q", d.TargetName, "local")
	}
	if got := len(d.Entries); got != 1 {
		t.Fatalf("Entries: got %d, want 1", got)
	}
	entry := d.Entries[0]
	if entry.Kind != "symlink" {
		t.Errorf("Entries[0].Kind: got %q, want %q", entry.Kind, "symlink")
	}
	if entry.Desc != "link config" {
		t.Errorf("Entries[0].Desc: got %q, want %q", entry.Desc, "link config")
	}
	if len(entry.Fields) == 0 {
		t.Fatal("Entries[0].Fields: got 0, want > 0 (symlink op is inspectable)")
	}
	wantFields := map[string]string{
		"target": "/etc/app.conf",
		"link":   "/etc/current.conf",
	}
	got := make(map[string]string, len(entry.Fields))
	for _, f := range entry.Fields {
		got[f.Label] = f.Value
	}
	for k, want := range wantFields {
		if got[k] != want {
			t.Errorf("field %q: got %q, want %q", k, got[k], want)
		}
	}
}

// TestInspectList_MultiDeploy_SortedByName covers the contract that
// InspectList returns details sorted by DeployName so callers render
// deterministically.
func TestInspectList_MultiDeploy_SortedByName(t *testing.T) {
	cfg := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "zulu", targets = [host]) {
  posix.symlink {
    desc   = "z"
    target = "/z"
    link   = "/zlink"
  }
}

std.deploy(name = "alpha", targets = [host]) {
  posix.symlink {
    desc   = "a"
    target = "/a"
    link   = "/alink"
  }
}

std.deploy(name = "mike", targets = [host]) {
  posix.symlink {
    desc   = "m"
    target = "/m"
    link   = "/mlink"
  }
}
`
	path := writeInspectCfg(t, cfg)
	store := diagnostic.NewSourceStore()
	em := diagnostic.NewEmitter(diagnostic.Policy{}, &harness.RecordingDisplayer{})

	details, err := engine.InspectList(diagnostic.NewCtx(t.Context(), em), path, store, spec.ResolveOptions{})
	if err != nil {
		t.Fatalf("engine.InspectList: %v", err)
	}

	want := []string{"alpha", "mike", "zulu"}
	if got := len(details); got != len(want) {
		t.Fatalf("details: got %d, want %d", got, len(want))
	}
	for i, w := range want {
		if details[i].DeployName != w {
			t.Errorf("details[%d].DeployName: got %q, want %q", i, details[i].DeployName, w)
		}
	}
}

// TestInspectList_PosixRun covers a step whose op implements
// OpInspector with apply/check fields. Asserts the resolved
// command strings make it into the entry's Fields.
func TestInspectList_PosixRun(t *testing.T) {
	cfg := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "solo", targets = [host]) {
  posix.run {
    desc  = "build app"
    check = "test -f /tmp/built"
    apply = "make build"
  }
}
`
	path := writeInspectCfg(t, cfg)
	store := diagnostic.NewSourceStore()
	em := diagnostic.NewEmitter(diagnostic.Policy{}, &harness.RecordingDisplayer{})

	details, err := engine.InspectList(diagnostic.NewCtx(t.Context(), em), path, store, spec.ResolveOptions{})
	if err != nil {
		t.Fatalf("engine.InspectList: %v", err)
	}

	if got := len(details); got != 1 {
		t.Fatalf("details: got %d, want 1", got)
	}
	if got := len(details[0].Entries); got != 1 {
		t.Fatalf("Entries: got %d, want 1", got)
	}
	got := make(map[string]string)
	for _, f := range details[0].Entries[0].Fields {
		got[f.Label] = f.Value
	}
	if got["apply"] != "make build" {
		t.Errorf("apply: got %q, want %q", got["apply"], "make build")
	}
	if got["check"] != "test -f /tmp/built" {
		t.Errorf("check: got %q, want %q", got["check"], "test -f /tmp/built")
	}
}
