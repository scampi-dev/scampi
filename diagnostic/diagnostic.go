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

// Raise constructs an event.Diagnostic from a Diagnostic value.
// Producers wanting to stamp a Cause should either set the field on
// the returned event or wrap the emitter via WithCause.
func Raise(d Diagnostic) event.Diagnostic {
	return event.Diagnostic{
		Time:     time.Now(),
		Severity: d.Severity(),
		Template: d.EventTemplate(),
	}
}
