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
units: []`,

		// invalid units shape
		`package fuzz
units: {}`,

		// missing fields
		`package fuzz
import "godoit.dev/doit/builtin"
units: [builtin.copy & { src: "a", dest: "b" }]`,

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
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC on user input:\n%q\npanic: %v", input, r)
			}
		}()

		err := engine.ApplyWithEnv(context.Background(), em, "/config.cue", store, src, tgt)
		// ---- Error classification invariant ----
		if err != nil {
			var abort engine.AbortError
			if !errors.As(err, &abort) {
				t.Fatalf("unexpected error type %T: %v", err, err)
			}
		}

		// ---- Event invariants ----
		for _, ev := range rec.events {
			assertEventWellFormed(t, ev)
		}
	})
}

func assertEventWellFormed(t *testing.T, ev event.Event) {
	t.Helper()

	// Kind must be known
	if ev.Kind.String() == "" {
		t.Fatalf("event has empty Kind: %#v", ev)
	}

	// Scope must be known
	if ev.Scope.String() == "" {
		t.Fatalf("event has empty Scope: %#v", ev)
	}

	// Severity must be known
	if ev.Severity.String() == "" {
		t.Fatalf("event has empty Severity: %#v", ev)
	}

	// DiagnosticRaised must carry a template
	if ev.Kind == event.DiagnosticRaised {
		d, ok := ev.Detail.(event.DiagnosticDetail)
		if !ok {
			t.Fatalf("DiagnosticRaised without DiagnosticDetail: %#v", ev)
		}
		if d.Template.ID == "" {
			t.Fatalf("DiagnosticRaised with empty Template.ID: %#v", ev)
		}
	}
}
