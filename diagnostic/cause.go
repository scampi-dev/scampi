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
