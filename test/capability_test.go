// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"errors"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
)

func assertCapabilityMismatch(t *testing.T, cfgStr string, tgt target.Target) {
	t.Helper()

	src := source.NewMemSource()
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

	var capErr engine.AbortError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}

	diagIDs := rec.collectDiagnosticIDs()
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
target.local(name="local")
deploy(
    name="test",
    targets=["local"],
    steps=[pkg(packages=["foo"], state="latest", source=system())],
)
`, newPkgOnlyTarget())
}

func TestPlan_Symlink_RequiresFilesystem(t *testing.T) {
	assertCapabilityMismatch(t, `
target.local(name="local")
deploy(
    name="test",
    targets=["local"],
    steps=[symlink(target="/opt/app/config.yaml", link="/etc/app/config.yaml")],
)
`, newSymlinkOnlyTarget())
}

func TestPlan_Run_RequiresCommand(t *testing.T) {
	assertCapabilityMismatch(t, `
target.local(name="local")
deploy(
    name="test",
    targets=["local"],
    steps=[run(apply="echo hello", check="true")],
)
`, newNoCommandTarget())
}

func TestPlan_Copy_RequiresOwnership(t *testing.T) {
	assertCapabilityMismatch(t, `
target.local(name="local")
deploy(
    name="test",
    targets=["local"],
    steps=[copy(src=local("/a"), dest="/b", perm="0644", owner="user", group="group")],
)
`, newMinimalTarget())
}
