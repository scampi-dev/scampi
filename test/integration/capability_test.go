// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"errors"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/test/harness"
)

func assertCapabilityMismatch(t *testing.T, cfgStr string, tgt target.Target) {
	t.Helper()

	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("engine.LoadConfig() must not return error, got %v", err)
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("engine.Resolve() must not return error, got %v", err)
	}

	resolved.Target = harness.MockTargetInstance(tgt)

	e, err := engine.New(diagnostic.NewCtx(ctx, em), src, resolved)
	if err != nil {
		t.Fatalf("engine.New() must not return error, got %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(ctx, em))

	var capErr engine.AbortError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}

	diagIDs := rec.CollectDiagnosticIDs()
	if len(diagIDs) == 0 {
		t.Fatal("expected at least 1 CapabilityMismatch diagnostic, got none")
	}
	for _, id := range diagIDs {
		if id != "engine.CapabilityMismatch" {
			t.Fatalf("expected engine.CapabilityMismatch diagnostic, got %q", id)
		}
	}
}

func TestPlan_PkgLatest_RequiresPkgUpdate(t *testing.T) {
	assertCapabilityMismatch(t, `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.pkg {
    packages = ["foo"]
    state = posix.PkgState.latest
    source = posix.pkg_system {}
  }
}
`, harness.NewPkgOnlyTarget())
}

func TestPlan_Symlink_RequiresFilesystem(t *testing.T) {
	assertCapabilityMismatch(t, `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.symlink { target = "/opt/app/config.yaml", link = "/etc/app/config.yaml" }
}
`, harness.NewSymlinkOnlyTarget())
}

func TestPlan_Run_RequiresCommand(t *testing.T) {
	assertCapabilityMismatch(t, `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.run { apply = "echo hello", check = "true" }
}
`, harness.NewNoCommandTarget())
}

func TestPlan_Copy_RequiresOwnership(t *testing.T) {
	assertCapabilityMismatch(t, `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    src = posix.source_local { path = "/a" }
    dest = "/b"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`, harness.NewMinimalTarget())
}
