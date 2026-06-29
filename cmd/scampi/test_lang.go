// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/linker"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/testkit"
)

// runLangTestFile runs a single scampi test file end-to-end:
// link → resolve → apply against mock targets → verify each
// registered mock against its declared `expect` field. Mismatches
// turn into TestFail diagnostics; clean apply + clean verify counts
// as one passing test per mock entry. Designed to mirror runTestFile
// (the legacy evaluation path) so the per-file dispatch in test.go
// can call either one transparently.
func runLangTestFile(
	ctx diagnostic.Ctx,
	testPath string,
	src source.Source,
) (passed, failed int, err error) {
	tests := testkit.NewTestRegistry()
	reg := testkit.NewEngineRegistry(engine.NewRegistry(), tests)

	cfg, err := linker.LoadConfig(ctx, testPath, src, reg)
	if err != nil {
		return 0, 0, err
	}

	resolved, err := engine.ResolveMultiple(cfg, spec.ResolveOptions{})
	if err != nil {
		return 0, 0, err
	}

	for _, rc := range resolved {
		e, engineErr := engine.New(ctx, src, rc)
		if engineErr != nil {
			return 0, 0, engineErr
		}
		if _, applyErr := e.Apply(ctx); applyErr != nil {
			e.Close()
			return 0, 0, applyErr
		}
		e.Close()
	}

	// Verify each registered mock against its declared expect.
	// Each mock counts as one logical test: clean verify = pass,
	// any mismatches = fail (with one diagnostic per mismatch so
	// the user sees every problem in one run).
	for _, entry := range tests.MemTargets() {
		mismatches := testkit.VerifyMemTarget(entry.Expect, entry.Mock)
		if len(mismatches) == 0 {
			passed++
			continue
		}
		failed++
		for _, m := range mismatches {
			ctx.Raise(&testkit.TestFail{
				Description: entry.Name + ": " + m.Key,
				Expected:    m.Matcher,
				Actual:      m.Reason,
			})
		}
	}

	return passed, failed, nil
}
