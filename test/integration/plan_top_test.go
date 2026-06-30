// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/test/harness"
)

// writePlanCfg writes cfg to a fresh tempdir as config.scampi and
// returns its path. Caller hands the path to engine.Plan.
func writePlanCfg(t *testing.T, cfg string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.scampi")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	return path
}

// TestPlan_SingleDeploy_Trivial covers the base case: one deploy with
// no cross-deploy edges. PlanResult should have one level with one
// node and HasGraph()==false.
func TestPlan_SingleDeploy_Trivial(t *testing.T) {
	cfg := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "solo", targets = [host]) {
  posix.run {
    desc  = "only step"
    check = "true"
    apply = "true"
  }
}
`
	path := writePlanCfg(t, cfg)
	store := diagnostic.NewSourceStore()
	em := diagnostic.NewEmitter(diagnostic.Policy{}, &harness.RecordingDisplayer{})

	result, err := engine.Plan(diagnostic.NewCtx(t.Context(), em), path, store, spec.ResolveOptions{})
	if err != nil {
		t.Fatalf("engine.Plan: %v", err)
	}

	if got := len(result.Levels); got != 1 {
		t.Fatalf("levels: got %d, want 1", got)
	}
	if got := len(result.Levels[0].Nodes); got != 1 {
		t.Fatalf("level[0].Nodes: got %d, want 1", got)
	}
	n := result.Levels[0].Nodes[0]
	if n.DeployName != "solo" {
		t.Errorf("DeployName: got %q, want %q", n.DeployName, "solo")
	}
	if len(n.After) != 0 {
		t.Errorf("After: got %v, want nil", n.After)
	}
	if len(n.Needs) != 0 {
		t.Errorf("Needs: got %v, want nil", n.Needs)
	}
	if got := len(n.Detail.Steps); got != 1 {
		t.Fatalf("Detail.Steps: got %d, want 1", got)
	}
	if got := n.Detail.Steps[0].Desc; got != "only step" {
		t.Errorf("step desc: got %q, want %q", got, "only step")
	}
	if result.HasGraph() {
		t.Errorf("HasGraph: got true, want false for single deploy with no edges")
	}
}

// TestPlan_MultiDeploy_LinearChain covers A -> B -> C ordering via
// label promises. Each level should hold one deploy, edges should
// point upstream by name, and Needs should expose the label that
// drove each edge.
func TestPlan_MultiDeploy_LinearChain(t *testing.T) {
	cfg := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "base", targets = [host]) {
  posix.run {
    desc     = "base"
    check    = "true"
    apply    = "true"
    promises = ["ready"]
  }
}

std.deploy(name = "middle", targets = [host]) {
  posix.run {
    desc     = "middle"
    check    = "true"
    apply    = "true"
    inputs   = ["ready"]
    promises = ["configured"]
  }
}

std.deploy(name = "app", targets = [host]) {
  posix.run {
    desc   = "app"
    check  = "true"
    apply  = "true"
    inputs = ["configured"]
  }
}
`
	path := writePlanCfg(t, cfg)
	store := diagnostic.NewSourceStore()
	em := diagnostic.NewEmitter(diagnostic.Policy{}, &harness.RecordingDisplayer{})

	result, err := engine.Plan(diagnostic.NewCtx(t.Context(), em), path, store, spec.ResolveOptions{})
	if err != nil {
		t.Fatalf("engine.Plan: %v", err)
	}

	if got := len(result.Levels); got != 3 {
		t.Fatalf("levels: got %d, want 3", got)
	}
	wantPerLevel := []struct {
		name  string
		after []string
		needs []string
	}{
		{"base", nil, nil},
		{"middle", []string{"base"}, []string{"[label]ready"}},
		{"app", []string{"middle"}, []string{"[label]configured"}},
	}
	for i, want := range wantPerLevel {
		nodes := result.Levels[i].Nodes
		if got := len(nodes); got != 1 {
			t.Fatalf("level[%d].Nodes: got %d, want 1", i, got)
		}
		n := nodes[0]
		if n.DeployName != want.name {
			t.Errorf("level[%d].Nodes[0].DeployName: got %q, want %q", i, n.DeployName, want.name)
		}
		assertStringSlice(t, "After", i, n.After, want.after)
		assertStringSlice(t, "Needs", i, n.Needs, want.needs)
	}
	if !result.HasGraph() {
		t.Errorf("HasGraph: got false, want true for linear chain")
	}
}

// TestPlan_MultiDeploy_FanOut covers A -> {B, C}: two deploys at
// level 1 are concurrent siblings under one root.
func TestPlan_MultiDeploy_FanOut(t *testing.T) {
	cfg := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "base", targets = [host]) {
  posix.run {
    desc     = "base"
    check    = "true"
    apply    = "true"
    promises = ["ready"]
  }
}

std.deploy(name = "left", targets = [host]) {
  posix.run {
    desc   = "left"
    check  = "true"
    apply  = "true"
    inputs = ["ready"]
  }
}

std.deploy(name = "right", targets = [host]) {
  posix.run {
    desc   = "right"
    check  = "true"
    apply  = "true"
    inputs = ["ready"]
  }
}
`
	path := writePlanCfg(t, cfg)
	store := diagnostic.NewSourceStore()
	em := diagnostic.NewEmitter(diagnostic.Policy{}, &harness.RecordingDisplayer{})

	result, err := engine.Plan(diagnostic.NewCtx(t.Context(), em), path, store, spec.ResolveOptions{})
	if err != nil {
		t.Fatalf("engine.Plan: %v", err)
	}

	if got := len(result.Levels); got != 2 {
		t.Fatalf("levels: got %d, want 2", got)
	}
	if got := len(result.Levels[0].Nodes); got != 1 || result.Levels[0].Nodes[0].DeployName != "base" {
		t.Fatalf("level[0]: got %+v, want [base]", result.Levels[0].Nodes)
	}
	// Level 1 has both children sorted by name (sort is stable in PlanResult).
	if got := len(result.Levels[1].Nodes); got != 2 {
		t.Fatalf("level[1].Nodes: got %d, want 2", got)
	}
	names := []string{result.Levels[1].Nodes[0].DeployName, result.Levels[1].Nodes[1].DeployName}
	want := []string{"left", "right"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("level[1].Nodes[%d].DeployName: got %q, want %q", i, names[i], want[i])
		}
	}
	for _, n := range result.Levels[1].Nodes {
		assertStringSlice(t, "After", 1, n.After, []string{"base"})
		assertStringSlice(t, "Needs", 1, n.Needs, []string{"[label]ready"})
	}
}

// TestPlan_DeployCycle_Errors covers A -> B -> A: two deploys
// promising/consuming each other's labels. Plan must return
// DeployCycleError without panicking.
func TestPlan_DeployCycle_Errors(t *testing.T) {
	cfg := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "a", targets = [host]) {
  posix.run {
    desc     = "a"
    check    = "true"
    apply    = "true"
    inputs   = ["from-b"]
    promises = ["from-a"]
  }
}

std.deploy(name = "b", targets = [host]) {
  posix.run {
    desc     = "b"
    check    = "true"
    apply    = "true"
    inputs   = ["from-a"]
    promises = ["from-b"]
  }
}
`
	path := writePlanCfg(t, cfg)
	store := diagnostic.NewSourceStore()
	em := diagnostic.NewEmitter(diagnostic.Policy{}, &harness.RecordingDisplayer{})

	_, err := engine.Plan(diagnostic.NewCtx(t.Context(), em), path, store, spec.ResolveOptions{})
	if err == nil {
		t.Fatal("engine.Plan: expected DeployCycleError, got nil")
	}
	var cyc engine.DeployCycleError
	if !errors.As(err, &cyc) {
		t.Errorf("engine.Plan: expected DeployCycleError, got %T: %v", err, err)
	}
}

func assertStringSlice(t *testing.T, label string, level int, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("level[%d].%s: got %v, want %v", level, label, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("level[%d].%s[%d]: got %q, want %q", level, label, i, got[i], want[i])
		}
	}
}
