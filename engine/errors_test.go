// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/signal"
)

type stubDiagnostic struct {
	id       errs.Code
	text     string
	severity signal.Severity
	impact   diagnostic.Impact
}

func (d stubDiagnostic) EventTemplate() event.Template {
	return event.Template{ID: d.id, Text: d.text, Data: d}
}

func (d stubDiagnostic) Severity() signal.Severity { return d.severity }
func (d stubDiagnostic) Impact() diagnostic.Impact { return d.impact }

func (d stubDiagnostic) Error() string {
	return fmt.Sprintf("diagnostic: %s", d.text)
}

// recEmitter is a local test-only emitter that captures every event.
type recEmitter struct {
	diagnostics []event.Diagnostic
	events      []event.Event
}

func (r *recEmitter) Emit(e event.Event) {
	r.events = append(r.events, e)
}

func (r *recEmitter) Raise(err diagnostic.Raisable) {
	r.Emit(err.Diagnostic())
}

func (r *recEmitter) EmitDiagnostic(e event.Diagnostic) {
	r.diagnostics = append(r.diagnostics, e)
}

func (r *recEmitter) EmitChange(event.Change)     {}
func (r *recEmitter) EmitProgress(event.Progress) {}

func TestEmitScopedDiagnostic_Nil(t *testing.T) {
	rec := &recEmitter{}
	impact, ok := emitScopedDiagnostic(rec, nil)

	if ok {
		t.Error("expected ok=false for nil error")
	}
	if impact != 0 {
		t.Errorf("expected impact=0, got %d", impact)
	}
	if len(rec.diagnostics) != 0 {
		t.Error("expected no diagnostics emitted")
	}
}

func TestEmitScopedDiagnostic_SingleDiagnostic(t *testing.T) {
	d := stubDiagnostic{
		id:       "test.single",
		text:     "single diagnostic",
		severity: signal.Error,
		impact:   diagnostic.ImpactAbort,
	}

	rec := &recEmitter{}
	impact, ok := emitScopedDiagnostic(rec, d)

	if !ok {
		t.Error("expected ok=true")
	}
	if impact != diagnostic.ImpactAbort {
		t.Errorf("expected ImpactAbort, got %d", impact)
	}
	if len(rec.diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic emitted, got %d", len(rec.diagnostics))
	}
	if rec.diagnostics[0].Template.ID != "test.single" {
		t.Errorf("wrong template ID: %q", rec.diagnostics[0].Template.ID)
	}
}

func TestEmitScopedDiagnostic_DiagnosticsSlice(t *testing.T) {
	ds := diagnostic.Diagnostics{
		stubDiagnostic{id: "d1", text: "first", severity: signal.Warning, impact: diagnostic.ImpactNone},
		stubDiagnostic{id: "d2", text: "second", severity: signal.Error, impact: diagnostic.ImpactAbort},
	}

	rec := &recEmitter{}
	impact, ok := emitScopedDiagnostic(rec, ds)

	if !ok {
		t.Error("expected ok=true")
	}
	if impact != diagnostic.ImpactAbort {
		t.Errorf("expected max impact ImpactAbort, got %d", impact)
	}
	if len(rec.diagnostics) != 2 {
		t.Fatalf("expected 2 diagnostics emitted, got %d", len(rec.diagnostics))
	}
}

func TestEmitScopedDiagnostic_PlainError(t *testing.T) {
	plain := fmt.Errorf("just a plain error")
	rec := &recEmitter{}
	impact, ok := emitScopedDiagnostic(rec, plain)

	if ok {
		t.Error("expected ok=false for plain error")
	}
	if impact != 0 {
		t.Errorf("expected impact=0, got %d", impact)
	}
	if len(rec.diagnostics) != 0 {
		t.Error("expected no diagnostics emitted")
	}
}

// TestEmitScopedDiagnostic_Raisable exercises errors that implement
// diagnostic.Raisable: they flow through em.Emit as event.Error, and
// the engine reads Impact off the struct.
func TestEmitScopedDiagnostic_Raisable(t *testing.T) {
	rec := &recEmitter{}
	err := stubRaisable{
		impact: event.ImpactAbort,
		tmpl:   event.Template{ID: "test.raisable", Text: "raisable diag"},
	}
	impact, ok := emitScopedDiagnostic(rec, err)

	if !ok {
		t.Fatal("expected ok=true")
	}
	if impact != diagnostic.ImpactAbort {
		t.Errorf("expected ImpactAbort, got %d", impact)
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	ev, isErr := rec.events[0].(event.Error)
	if !isErr {
		t.Fatalf("expected event.Error, got %T", rec.events[0])
	}
	if ev.Template.ID != "test.raisable" {
		t.Errorf("wrong template ID: %q", ev.Template.ID)
	}
	if ev.Impact != event.ImpactAbort {
		t.Errorf("wrong Impact: %v", ev.Impact)
	}
}

type stubRaisable struct {
	impact event.Impact
	tmpl   event.Template
}

func (s stubRaisable) Error() string { return "stub raisable" }
func (s stubRaisable) Diagnostic() event.Event {
	return event.Error{Impact: s.impact, Template: s.tmpl}
}
