// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/spec"
)

// Package-level Check drives the full multi-deploy converge: config load,
// resolve, the level graph, and report aggregation. The config has three
// deploys: "alpha" (slow, promises shared:ready), "beta" (fast, independent),
// and "gamma" (consumes shared:ready, so level 1). All checks are satisfied
// no-ops against the local target.
const multiDeployConfig = `
module main

import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "alpha", targets = [host]) {
  posix.run {
    desc     = "alpha step"
    check    = "sleep 0.2"
    apply    = "true"
    promises = ["shared:ready"]
  }
}

std.deploy(name = "beta", targets = [host]) {
  posix.run { desc = "beta step", check = "true", apply = "true" }
}

std.deploy(name = "gamma", targets = [host]) {
  posix.run {
    desc   = "gamma step"
    check  = "true"
    apply  = "true"
    inputs = ["shared:ready"]
  }
}
`

// Check's aggregate report lists steps in lane-ordinal order (level-major,
// declaration order within a level), not deploy completion order: "beta"
// finishes long before the sleeping "alpha", and "gamma" only runs in level 1,
// yet the report reads alpha, beta, gamma on every run (#440, #445).
func Test_Check_AggregatesMultiDeployInOrdinalOrder(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.scampi")
	if err := os.WriteFile(cfgPath, []byte(multiDeployConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	em := diagnostic.NewEmitter(diagnostic.Policy{}, diagnostic.Discard{})
	ctx := diagnostic.NewCtx(t.Context(), em)
	rep, err := Check(ctx, cfgPath, nil, spec.ResolveOptions{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	var descs []string
	for _, sr := range rep.Steps {
		descs = append(descs, sr.Step.Desc())
	}
	want := []string{"alpha step", "beta step", "gamma step"}
	if len(descs) != len(want) {
		t.Fatalf("aggregate report has %d steps, want %d (%v)", len(descs), len(want), descs)
	}
	for i, w := range want {
		if descs[i] != w {
			t.Fatalf("report order = %v, want %v", descs, want)
		}
	}
}
