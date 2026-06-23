// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
)

// recEmitter is a local test-only emitter that captures every event.
type recEmitter struct {
	events []event.Event
}

func (r *recEmitter) Emit(e event.Event) {
	r.events = append(r.events, e)
}

func (r *recEmitter) Raise(err diagnostic.Raisable) {
	r.Emit(err.Diagnostic())
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
	if len(rec.events) != 0 {
		t.Error("expected no events emitted")
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
	if len(rec.events) != 0 {
		t.Error("expected no events emitted")
	}
}

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
