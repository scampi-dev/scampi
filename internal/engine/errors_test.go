// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"testing"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
)

// recOutput is a local test Output that captures every event. Embeds Discard
// for the one-shot no-ops; wrap it in NewEmitter to feed a Ctx.
type recOutput struct {
	diagnostic.Discard
	events []event.Event
}

func (r *recOutput) RenderEvent(e event.Event) {
	r.events = append(r.events, e)
}

// recCtx returns a Ctx backed by rec, so a test can drive a helper that takes
// a Ctx and then inspect rec.events.
func recCtx(t *testing.T, rec *recOutput) diagnostic.Ctx {
	return diagnostic.NewCtx(t.Context(), diagnostic.NewEmitter(diagnostic.Policy{}, rec))
}

func TestEmitScopedDiagnostic_Nil(t *testing.T) {
	rec := &recOutput{}
	impact, ok := emitScopedDiagnostic(recCtx(t, rec), nil)

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
	rec := &recOutput{}
	impact, ok := emitScopedDiagnostic(recCtx(t, rec), plain)

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
	rec := &recOutput{}
	err := stubRaisable{
		impact: event.ImpactAbort,
		tmpl:   event.Template{ID: "test.raisable", Text: "raisable diag"},
	}
	impact, ok := emitScopedDiagnostic(recCtx(t, rec), err)

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
