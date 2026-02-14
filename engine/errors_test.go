// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
)

type stubDiagnostic struct {
	id       string
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

func TestEmitScopedDiagnostic_Nil(t *testing.T) {
	var emitted []diagnostic.Diagnostic
	impact, ok := emitScopedDiagnostic(nil, func(d diagnostic.Diagnostic) {
		emitted = append(emitted, d)
	})

	if ok {
		t.Error("expected ok=false for nil error")
	}
	if impact != 0 {
		t.Errorf("expected impact=0, got %d", impact)
	}
	if len(emitted) != 0 {
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

	var emitted []diagnostic.Diagnostic
	impact, ok := emitScopedDiagnostic(d, func(d diagnostic.Diagnostic) {
		emitted = append(emitted, d)
	})

	if !ok {
		t.Error("expected ok=true")
	}
	if impact != diagnostic.ImpactAbort {
		t.Errorf("expected ImpactAbort, got %d", impact)
	}
	if len(emitted) != 1 {
		t.Fatalf("expected 1 diagnostic emitted, got %d", len(emitted))
	}
	if emitted[0].EventTemplate().ID != "test.single" {
		t.Errorf("wrong template ID: %q", emitted[0].EventTemplate().ID)
	}
}

func TestEmitScopedDiagnostic_DiagnosticsSlice(t *testing.T) {
	ds := diagnostic.Diagnostics{
		stubDiagnostic{id: "d1", text: "first", severity: signal.Warning, impact: diagnostic.ImpactNone},
		stubDiagnostic{id: "d2", text: "second", severity: signal.Error, impact: diagnostic.ImpactAbort},
	}

	var emitted []diagnostic.Diagnostic
	impact, ok := emitScopedDiagnostic(ds, func(d diagnostic.Diagnostic) {
		emitted = append(emitted, d)
	})

	if !ok {
		t.Error("expected ok=true")
	}
	if impact != diagnostic.ImpactAbort {
		t.Errorf("expected max impact ImpactAbort, got %d", impact)
	}
	if len(emitted) != 2 {
		t.Fatalf("expected 2 diagnostics emitted, got %d", len(emitted))
	}
}

func TestEmitScopedDiagnostic_PlainError(t *testing.T) {
	plain := fmt.Errorf("just a plain error")

	var emitted []diagnostic.Diagnostic
	impact, ok := emitScopedDiagnostic(plain, func(d diagnostic.Diagnostic) {
		emitted = append(emitted, d)
	})

	if ok {
		t.Error("expected ok=false for plain error")
	}
	if impact != 0 {
		t.Errorf("expected impact=0, got %d", impact)
	}
	if len(emitted) != 0 {
		t.Error("expected no diagnostics emitted")
	}
}
