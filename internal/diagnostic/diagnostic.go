// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"reflect"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/spec"
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

// Impact and its constants are aliases for the canonical types in
// event/. The diagnostic package re-exports them so producers that
// already import diagnostic don't also have to import event just to
// reference Impact.
type Impact = event.Impact

const (
	ImpactNone  = event.ImpactNone
	ImpactAbort = event.ImpactAbort
)

// Deferrable is implemented by errors that reference a missing resource which
// could be created by an upstream action. The engine uses this to defer aborts
// during check mode when the resource is already promised.
type Deferrable interface {
	DeferredResource() spec.Resource
}

type Emitter interface {
	// Emit accepts any event.Event - the sealed union of Error,
	// Warning, Info, Change, Progress. Use Emit when the event has
	// already been constructed.
	Emit(event.Event)
	// Raise emits the event produced by a Raisable error. The helper
	// resolves err.Diagnostic() and forwards it through Emit, so call
	// sites stay close to the error site instead of constructing the
	// event by hand.
	Raise(err Raisable)
}

// Raisable is implemented by errors that map directly onto a
// renderable event (event.Error, event.Warning, or event.Info).
// Emitter.Raise routes Raisable errors through Emit; the engine
// type-switches on the returned concrete type to decide whether the
// diagnostic aborts execution (only event.Error carries Impact).
type Raisable interface {
	error
	Diagnostic() event.Event
}
