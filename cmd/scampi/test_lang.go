// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/linker"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/testkit"
)

// runLangTestFile runs a single scampi-lang test file end-to-end:
// link → resolve → apply against mock targets → verify each
// registered mock against its declared `expect` field. Mismatches
// turn into TestFail diagnostics; clean apply + clean verify counts
// as one passing test per mock entry. Designed to mirror runTestFile
// (the legacy Starlark path) so the per-file dispatch in test.go
// can call either one transparently.
func runLangTestFile(
	ctx context.Context,
	em diagnostic.Emitter,
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
		e, engineErr := engine.New(ctx, src, rc, em)
		if engineErr != nil {
			return 0, 0, engineErr
		}
		if applyErr := e.Apply(ctx); applyErr != nil {
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
			em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic(
				testPath,
				&testkit.TestFail{
					Description: entry.Name + ": " + m.Key,
					Expected:    m.Matcher,
					Actual:      m.Reason,
				},
			))
		}
	}

	// REST mock verification: walk each registered REST mock and
	// run its expect_requests matchers against the recorded calls.
	// Each mock counts as one logical test.
	for _, entry := range tests.MemRESTs() {
		mismatches := testkit.VerifyMemREST(entry.ExpectRequests, entry.Mock)
		if len(mismatches) == 0 {
			passed++
			continue
		}
		failed++
		for _, m := range mismatches {
			em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic(
				testPath,
				&testkit.TestFail{
					Description: entry.Name + ": " + m.Key,
					Expected:    m.Matcher,
					Actual:      m.Reason,
				},
			))
		}
	}

	return passed, failed, nil
}

// detectLangSyntax reports whether the given source is a scampi-lang
// file. The check is "first non-empty, non-comment line starts with
// `module `" — scampi-lang requires a module declaration as the
// first significant token; legacy Starlark test files don't have one.
func detectLangSyntax(data []byte) bool {
	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			continue
		}
		return strings.HasPrefix(trimmed, "module ")
	}
	return false
}
