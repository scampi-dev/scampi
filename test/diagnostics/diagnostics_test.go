// SPDX-License-Identifier: GPL-3.0-only

// Scope: cross-package data-driven regression suite over a catalog of
// scampi configs (test/testdata/diagnostics/<scenario>/). Each fixture
// has a config.scampi + expect.json (or expect-<format>.json); the
// harness loads, plans, applies against a MemTarget, and asserts the
// recorded diagnostics match.
//
// Exercises: the full lang → engine → diagnostic pipeline. Anchors
// diagnostic IDs, severities, source spans, and scopes against regressions.
//
// Snapshot regen: set SCAMPI_UPDATE_DIAGNOSTICS=1 to rewrite every
// expect.json from the recording. Use after intentional changes to
// error IDs, scopes, or source spans; review the diff before committing.

package diagnostics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/test/harness"
)

func TestDiagnostics(t *testing.T) {
	root := harness.AbsPath("../testdata/diagnostics")

	entries := harness.ReadDirOrDie(root)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		name := e.Name()
		dir := filepath.Join(root, name)

		cfgPath := filepath.Join(dir, "config.scampi")
		if _, err := harness.ReadFileSafe(cfgPath); err != nil {
			t.Errorf("%s: no config.scampi found", name)
			continue
		}

		t.Run(name, func(t *testing.T) {
			runDiagnosticsCase(t, dir, "config.scampi", "scampi")
		})
	}
}

func runDiagnosticsCase(t *testing.T, dir string, cfgFilename string, format string) {
	cfgPath := filepath.Join(dir, cfgFilename)

	// Prefer format-specific expect file, fall back to default
	expectPath := filepath.Join(dir, "expect-"+format+".json")
	if _, err := harness.ReadFileSafe(expectPath); err != nil {
		expectPath = filepath.Join(dir, "expect.json")
	}

	src := source.LocalPosixSource{}
	tgt := target.NewMemTarget()

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()

	apply := func() error {
		cfg, err := engine.LoadConfig(ctx, em, cfgPath, store, src)
		if err != nil {
			return err
		}

		resolved, err := engine.Resolve(cfg, "", "")
		if err != nil {
			return err
		}

		resolved.Target = harness.MockTargetInstance(tgt)

		e, err := engine.New(ctx, src, resolved, em)
		if err != nil {
			return err
		}
		defer e.Close()

		return e.Apply(ctx)
	}

	err := apply()

	var abort engine.AbortError
	abortOccurred := errors.As(err, &abort)

	if os.Getenv("SCAMPI_UPDATE_DIAGNOSTICS") == "1" {
		harness.WriteSnapshot(t, expectPath, abortOccurred, rec, cfgPath)
		return
	}

	expect := harness.LoadExpected(t, expectPath)
	if expect.Abort {
		if !abortOccurred {
			t.Fatalf("expected AbortError, got %v", err)
		}
	} else if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() {
		if t.Failed() {
			rec.Dump(t.Output())
		}
	}()
	harness.AssertDiagnostics(t, rec, expect.Diagnostics, cfgPath)
}
