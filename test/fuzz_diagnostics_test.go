package test

import (
	"context"
	"errors"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

func FuzzDiagnostics(f *testing.F) {
	// ---- Seeds: real, high-value starting points ----

	seeds := []string{
		// minimal valid-ish
		`package fuzz
steps: []`,

		// invalid steps shape
		`package fuzz
steps: {}`,

		// missing copy fields
		`package fuzz
import "godoit.dev/doit/builtin"
steps: [builtin.copy & { src: "a", dest: "b" }]`,

		// missing template fields
		`package fuzz
import "godoit.dev/doit/builtin"
steps: [builtin.symlink & { target: "a" }]`,

		// missing template fields
		`package fuzz
import "godoit.dev/doit/builtin"
steps: [builtin.template & { src: "a", dest: "b" }]`,

		// garbage
		`this is not cue`,

		// empty
		``,
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		src := source.NewMemSource()
		tgt := target.NewMemTarget()

		src.Files["/config.cue"] = []byte(input)

		rec := &recordingDisplayer{}
		em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
		store := spec.NewSourceStore()

		// ---- Hard invariant: user input must not panic ----
		// CUE panics are caught by the engine and converted to CuePanic errors.
		// Any panic here indicates a bug in doit, not CUE.
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC on user input:\n%q\npanic: (%T) %v", input, r, r)
			}
		}()

		apply := func() error {
			ctx := context.Background()
			cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
			if err != nil {
				return err
			}

			cfg.Target = mockTargetInstance(tgt)

			e, err := engine.New(ctx, src, cfg, em)
			if err != nil {
				return err
			}
			defer e.Close()

			return e.Apply(ctx)
		}

		err := apply()
		// ---- Error classification invariant ----
		if err != nil {
			var abort engine.AbortError
			if !errors.As(err, &abort) {
				t.Fatalf("unexpected error type %T: %v", err, err)
			}
		}

		// ---- Diagnostic invariants ----
		assertEngineDiagnosticsWellFormed(t, rec.engineDiagnostics)
		assertPlanDiagnosticsWellFormed(t, rec.planDiagnostics)
		assertActionDiagnosticsWellFormed(t, rec.actionDiagnostics)
		assertOpDiagnosticsWellFormed(t, rec.opDiagnostics)
	})
}

func assertEngineDiagnosticsWellFormed(t *testing.T, diags []event.EngineDiagnostic) {
	t.Helper()
	for i, d := range diags {
		if d.Severity.String() == "" {
			t.Fatalf("engine diagnostic [%d] has empty Severity", i)
		}
		if d.Detail.Template.ID == "" {
			t.Fatalf("engine diagnostic [%d] has empty Template.ID", i)
		}
	}
}

func assertPlanDiagnosticsWellFormed(t *testing.T, diags []event.PlanDiagnostic) {
	t.Helper()
	for i, d := range diags {
		if d.Severity.String() == "" {
			t.Fatalf("plan diagnostic [%d] has empty Severity", i)
		}
		if d.Detail.Template.ID == "" {
			t.Fatalf("plan diagnostic [%d] has empty Template.ID", i)
		}
	}
}

func assertActionDiagnosticsWellFormed(t *testing.T, diags []event.ActionDiagnostic) {
	t.Helper()
	for i, d := range diags {
		if d.Severity.String() == "" {
			t.Fatalf("action diagnostic [%d] has empty Severity", i)
		}
		if d.Detail.Template.ID == "" {
			t.Fatalf("action diagnostic [%d] has empty Template.ID", i)
		}
	}
}

func assertOpDiagnosticsWellFormed(t *testing.T, diags []event.OpDiagnostic) {
	t.Helper()
	for i, d := range diags {
		if d.Severity.String() == "" {
			t.Fatalf("op diagnostic [%d] has empty Severity", i)
		}
		if d.Detail.Template.ID == "" {
			t.Fatalf("op diagnostic [%d] has empty Template.ID", i)
		}
	}
}
