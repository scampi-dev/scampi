// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import "scampi.dev/scampi/diagnostic/event"

// WithCause returns an Emitter that stamps the given Cause on every
// v2 event (Diagnostic, Change, Progress) forwarded through it.
// Events that already carry a non-zero Cause are passed through
// unchanged - explicit wins over the wrapper.
//
// Old lifecycle and *Diagnostic methods pass through untouched. They
// have no Cause field; this wrapper is a no-op for them.
//
// Typical use: wrap the emitter when entering a hook context so every
// emit inside the hook gets a CauseHook stamp.
//
//	hookEm := diagnostic.WithCause(emitter, event.Cause{
//	    Kind: event.CauseHook, Ref: hookID,
//	})
//	runHook(ctx, hookEm)
func WithCause(e Emitter, c event.Cause) Emitter {
	if c.Kind == event.CauseNone {
		return e
	}
	return &causeEmitter{inner: e, cause: c}
}

type causeEmitter struct {
	inner Emitter
	cause event.Cause
}

func (w *causeEmitter) stamp(c event.Cause) event.Cause {
	if c.Kind != event.CauseNone {
		return c // explicit cause wins over the wrapper
	}
	return w.cause
}

func (w *causeEmitter) EmitDiagnostic(e event.Diagnostic) {
	e.Cause = w.stamp(e.Cause)
	w.inner.EmitDiagnostic(e)
}

func (w *causeEmitter) EmitChange(e event.Change) {
	e.Cause = w.stamp(e.Cause)
	w.inner.EmitChange(e)
}

func (w *causeEmitter) EmitProgress(e event.Progress) {
	w.inner.EmitProgress(e)
}

// Old surface passes through. These methods stay until phase 6.

func (w *causeEmitter) EmitEngineLifecycle(e event.EngineEvent) { w.inner.EmitEngineLifecycle(e) }
func (w *causeEmitter) EmitPlanLifecycle(e event.PlanEvent)     { w.inner.EmitPlanLifecycle(e) }
func (w *causeEmitter) EmitActionLifecycle(e event.ActionEvent) { w.inner.EmitActionLifecycle(e) }
func (w *causeEmitter) EmitOpLifecycle(e event.OpEvent)         { w.inner.EmitOpLifecycle(e) }
func (w *causeEmitter) EmitIndexAll(e event.IndexAllEvent)      { w.inner.EmitIndexAll(e) }
func (w *causeEmitter) EmitIndexStep(e event.IndexStepEvent)    { w.inner.EmitIndexStep(e) }
func (w *causeEmitter) EmitInspect(e event.InspectEvent)        { w.inner.EmitInspect(e) }
func (w *causeEmitter) EmitGraph(e event.GraphEvent)            { w.inner.EmitGraph(e) }
func (w *causeEmitter) EmitEngineDiagnostic(e event.EngineDiagnostic) {
	w.inner.EmitEngineDiagnostic(e)
}
func (w *causeEmitter) EmitPlanDiagnostic(e event.PlanDiagnostic) { w.inner.EmitPlanDiagnostic(e) }
func (w *causeEmitter) EmitActionDiagnostic(e event.ActionDiagnostic) {
	w.inner.EmitActionDiagnostic(e)
}
func (w *causeEmitter) EmitOpDiagnostic(e event.OpDiagnostic) { w.inner.EmitOpDiagnostic(e) }
