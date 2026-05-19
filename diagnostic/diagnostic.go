// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=Impact
package diagnostic

import (
	"reflect"
	"strings"
	"time"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

// OpDisplayID derives a display identifier for an op.
// Uses OpDescriber template ID if available, otherwise falls back to type name.
func OpDisplayID(op spec.Op) string {
	if d, ok := op.(spec.OpDescriber); ok {
		if desc := d.OpDescription(); desc != nil {
			if id := desc.PlanTemplate().ID; id != "" {
				return string(id)
			}
		}
	}
	// Fallback: use the struct type name
	t := reflect.TypeOf(op)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

type Impact uint8

const (
	ImpactNone Impact = iota
	ImpactAbort
)

func (i Impact) ShouldAbort() bool {
	return i == ImpactAbort
}

// Deferrable is implemented by errors that reference a missing resource which
// could be created by an upstream action. The engine uses this to defer aborts
// during check mode when the resource is already promised.
type Deferrable interface {
	DeferredResource() spec.Resource
}

type (
	Emitter interface {
		// Emit accepts any event.Event - the sealed union of Error,
		// Warning, Info, Change, Progress. Use Emit when the event
		// has already been constructed.
		Emit(event.Event)
		// Raise emits the event produced by a Raisable error. The
		// helper resolves err.Diagnostic() and forwards it through
		// Emit, so call sites stay close to the error site instead
		// of constructing the event by hand.
		Raise(err Raisable)
		EmitDiagnostic(e event.Diagnostic)
		EmitChange(e event.Change)
		EmitProgress(e event.Progress)
	}
	Diagnostic interface {
		EventTemplate() event.Template
		Severity() signal.Severity
		Impact() Impact
	}
	// FatalError provides the Severity and Impact methods shared by all
	// diagnostic errors that abort execution: Severity=Error, Impact=Abort.
	// Embed it in error structs to avoid repeating these two methods.
	FatalError struct{}

	// Warning provides Severity=Warning, Impact=None for non-fatal diagnostics
	// that should be reported but don't stop execution.
	Warning struct{}

	// Info provides Severity=Info, Impact=None for informational diagnostics.
	Info struct{}

	Diagnostics     []Diagnostic
	MultiDiagnostic interface {
		Diagnostics() []Diagnostic
	}
)

func (FatalError) Severity() signal.Severity { return signal.Error }
func (FatalError) Impact() Impact            { return ImpactAbort }

func (Warning) Severity() signal.Severity { return signal.Warning }
func (Warning) Impact() Impact            { return ImpactNone }

func (Info) Severity() signal.Severity { return signal.Info }
func (Info) Impact() Impact            { return ImpactNone }

func (d Diagnostics) Diagnostics() []Diagnostic { return d }
func (d Diagnostics) Error() string {
	msgs := make([]string, len(d))
	for i, diag := range d {
		if e, ok := diag.(error); ok {
			msgs[i] = e.Error()
		} else {
			msgs[i] = diag.EventTemplate().Text
		}
	}
	return strings.Join(msgs, "; ")
}

// ActionDeps maps action index to indices of actions it depends on.
type ActionDeps [][]int

// Diagnostics
// -----------------------------------------------------------------------------

// Raisable is implemented by errors that map directly onto a
// renderable event (event.Error, event.Warning, or event.Info).
// Emitter.Raise routes Raisable errors through Emit; the engine
// type-switches on the returned concrete type to decide whether the
// diagnostic aborts execution (only event.Error carries Impact).
type Raisable interface {
	error
	Diagnostic() event.Event
}

// Raisables aggregates multiple Raisable errors into a single error
// value. Producers that run many checks in a batch return a Raisables
// so each entry surfaces individually through emitScopedDiagnostic.
type Raisables []Raisable

func (rs Raisables) Error() string {
	msgs := make([]string, len(rs))
	for i, r := range rs {
		msgs[i] = r.Error()
	}
	return strings.Join(msgs, "; ")
}

// RaiseLegacy bridges the old Diagnostic interface (with
// EventTemplate/Severity methods) to event.Diagnostic. It exists only
// for as long as the Diagnostic interface does; once every producer
// implements Raisable, this and the Diagnostic interface go away.
func RaiseLegacy(d Diagnostic) event.Diagnostic {
	return event.Diagnostic{
		Time:     time.Now(),
		Severity: d.Severity(),
		Template: d.EventTemplate(),
	}
}
