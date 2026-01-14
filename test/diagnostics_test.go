package test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

func TestDiagnostics(t *testing.T) {
	root := absPath("testdata/diagnostics")

	entries := readDirOrDie(root)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		name := e.Name()
		t.Run(name, func(t *testing.T) {
			runDiagnosticsCase(t, filepath.Join(root, name))
		})
	}
}

func runDiagnosticsCase(t *testing.T, dir string) {
	cfgPath := filepath.Join(dir, "config.cue")
	expectPath := filepath.Join(dir, "expect.json")

	expect := loadExpected(t, expectPath)

	recTgt := &target.Recorder{Inner: target.LocalPosixTarget{}}
	rec := &recordingDisplayer{}
	pol := diagnostic.Policy{}
	em := diagnostic.NewEmitter(pol, rec)
	store := spec.NewSourceStore()

	err := engine.ApplyWithEnv(context.Background(), em, cfgPath, store, source.LocalPosixSource{}, recTgt)

	if expect.Abort {
		var abort engine.AbortError
		if !errors.As(err, &abort) {
			t.Fatalf("expected AbortError, got %v", err)
		}
	} else if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer rec.dump(t.Output())
	assertDiagnostics(t, rec.events, expect.Diagnostics, cfgPath)

	AssertTargetUntouched(t, recTgt)
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

func assertDiagnostics(
	t *testing.T,
	have []event.Event,
	expect []ExpectedDiagnostic,
	cfgPath string,
) {
	var actual []event.Event
	for _, ev := range have {
		if ev.Kind == event.DiagnosticRaised {
			actual = append(actual, ev)
		}
	}

	if len(actual) != len(expect) {
		t.Fatalf("expected %d events, got %d", len(expect), len(actual))
	}

	for i, exp := range expect {
		// NOTE: diagnostics are expected to be emitted in deterministic order
		ev := actual[i]

		if ev.Severity.String() != exp.Severity {
			t.Fatalf("[%d] expected severity %q, got %q", i, exp.Severity, ev.Severity)
		}
		if ev.Kind.String() != exp.Kind {
			t.Fatalf("[%d] expected kind %q, got %q", i, exp.Kind, ev.Kind)
		}
		if ev.Scope.String() != exp.Scope {
			t.Fatalf("[%d] expected scope %q, got %q", i, exp.Scope, ev.Scope)
		}

		d := diagnosticDetail(t, ev)

		tmpl := d.Template

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
			if tmpl.Source.Line != exp.Source.Line {
				t.Fatalf("[%d] expected line %d, got %d", i, exp.Source.Line, tmpl.Source.Line)
			}
		}

		if exp.Unit != nil {
			if ev.Subject.Index != exp.Unit.Index {
				t.Fatalf("[%d] expected unit index %d, got %d", i, exp.Unit.Index, ev.Subject.Index)
			}
			if ev.Subject.Kind != exp.Unit.Kind {
				t.Fatalf("[%d] expected unit kind %q, got %q", i, exp.Unit.Kind, ev.Subject.Kind)
			}
		}
	}
}

func diagnosticDetail(t *testing.T, ev event.Event) event.DiagnosticDetail {
	t.Helper()

	if ev.Kind != event.DiagnosticRaised {
		t.Fatalf("expected DiagnosticRaised, got %v", ev.Kind)
	}

	d, ok := ev.Detail.(event.DiagnosticDetail)
	if !ok {
		t.Fatalf("unexpected Detail type %T", ev.Detail)
	}
	return d
}
