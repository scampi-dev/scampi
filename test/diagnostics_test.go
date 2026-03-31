// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
)

func TestDiagnostics(t *testing.T) {
	root := absPath("testdata/diagnostics")

	entries := readDirOrDie(root)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		name := e.Name()
		dir := filepath.Join(root, name)

		cfgPath := filepath.Join(dir, "config.scampi")
		if _, err := readFileSafe(cfgPath); err != nil {
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
	if _, err := readFileSafe(expectPath); err != nil {
		expectPath = filepath.Join(dir, "expect.json")
	}

	expect := loadExpected(t, expectPath)

	src := source.LocalPosixSource{}
	tgt := allCapNoImplTarget{}

	rec := &recordingDisplayer{}
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

		resolved.Target = mockTargetInstance(tgt)

		e, err := engine.New(ctx, src, resolved, em)
		if err != nil {
			return err
		}
		defer e.Close()

		return e.Apply(ctx)
	}

	err := apply()

	if expect.Abort {
		var abort engine.AbortError
		if !errors.As(err, &abort) {
			t.Fatalf("expected AbortError, got %v", err)
		}
	} else if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() {
		if t.Failed() {
			rec.dump(t.Output())
		}
	}()
	assertDiagnostics(t, rec, expect.Diagnostics, cfgPath)

	// AssertTargetUntouched(t, recTgt)
}

func loadExpected(t *testing.T, path string) ExpectedDiagnostics {
	t.Helper()

	data := readOrDie(path)
	var e ExpectedDiagnostics
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

// collectedDiagnostic is a normalized representation of any diagnostic type
// for easier comparison with expected diagnostics.
type collectedDiagnostic struct {
	scope    string
	severity string
	template event.Template
	step     *event.StepDetail
}

func collectDiagnostics(rec *recordingDisplayer) []collectedDiagnostic {
	var collected []collectedDiagnostic

	for _, d := range rec.engineDiagnostics {
		collected = append(collected, collectedDiagnostic{
			scope:    "ScopeEngine",
			severity: d.Severity.String(),
			template: d.Detail.Template,
			step:     nil,
		})
	}

	for _, d := range rec.planDiagnostics {
		step := d.Step
		collected = append(collected, collectedDiagnostic{
			scope:    "ScopePlan",
			severity: d.Severity.String(),
			template: d.Detail.Template,
			step:     &step,
		})
	}

	for _, d := range rec.actionDiagnostics {
		step := d.Step
		collected = append(collected, collectedDiagnostic{
			scope:    "ScopeAction",
			severity: d.Severity.String(),
			template: d.Detail.Template,
			step:     &step,
		})
	}

	for _, d := range rec.opDiagnostics {
		step := d.Step
		collected = append(collected, collectedDiagnostic{
			scope:    "ScopeOp",
			severity: d.Severity.String(),
			template: d.Detail.Template,
			step:     &step,
		})
	}

	return collected
}

func assertDiagnostics(
	t *testing.T,
	rec *recordingDisplayer,
	expect []ExpectedDiagnostic,
	cfgPath string,
) {
	t.Helper()

	actual := collectDiagnostics(rec)

	if len(actual) != len(expect) {
		t.Fatalf("expected %d diagnostics, got %d", len(expect), len(actual))
	}

	for i, exp := range expect {
		// NOTE: diagnostics are expected to be emitted in deterministic order
		got := actual[i]

		if got.severity != exp.Severity {
			t.Fatalf("[%d] expected severity %q, got %q", i, exp.Severity, got.severity)
		}

		// Kind is always "DiagnosticRaised" for diagnostics - implicit in the new model
		if exp.Kind != "DiagnosticRaised" {
			t.Fatalf("[%d] unexpected kind in test data: %q (should always be DiagnosticRaised)", i, exp.Kind)
		}

		if got.scope != exp.Scope {
			t.Fatalf("[%d] expected scope %q, got %q", i, exp.Scope, got.scope)
		}

		tmpl := got.template

		if tmpl.ID != exp.ID {
			t.Fatalf("[%d] expected id %q, got %q", i, exp.ID, tmpl.ID)
		}

		if exp.Source != nil {
			if tmpl.Source == nil {
				t.Fatalf("[%d] expected source, got nil", i)
			}
			if tmpl.Source.Filename != cfgPath {
				t.Fatalf("[%d] expected source file %q, got %q", i, cfgPath, tmpl.Source.Filename)
			}
			s := tmpl.Source
			e := exp.Source
			startMatch := s.StartLine == e.StartLine && s.StartCol == e.StartCol
			endMatch := s.EndLine == e.EndLine && s.EndCol == e.EndCol
			if !startMatch || !endMatch {
				t.Fatalf("[%d] source span mismatch:\n  got:  %d:%d → %d:%d\n  want: %d:%d → %d:%d",
					i,
					s.StartLine, s.StartCol, s.EndLine, s.EndCol,
					e.StartLine, e.StartCol, e.EndLine, e.EndCol,
				)
			}
		}

		if exp.Step != nil {
			if got.step == nil {
				t.Fatalf("[%d] expected step info, got nil", i)
			}
			if got.step.StepIndex != exp.Step.Index {
				t.Fatalf("[%d] expected step index %d, got %d", i, exp.Step.Index, got.step.StepIndex)
			}
			if got.step.StepKind != exp.Step.Kind {
				t.Fatalf("[%d] expected step kind %q, got %q", i, exp.Step.Kind, got.step.StepKind)
			}
		}
	}
}
