// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import "scampi.dev/scampi/diagnostic/event"

// WithCause returns an Emitter that stamps the given Cause on every
// event forwarded through it. Events that already carry a non-zero
// Cause pass through unchanged - explicit wins over the wrapper.
// Progress events have no Cause field and are forwarded as-is.
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

func (w *causeEmitter) Emit(e event.Event) {
	switch v := e.(type) {
	case event.Error:
		v.Cause = w.stamp(v.Cause)
		w.inner.Emit(v)
	case event.Warning:
		v.Cause = w.stamp(v.Cause)
		w.inner.Emit(v)
	case event.Info:
		v.Cause = w.stamp(v.Cause)
		w.inner.Emit(v)
	case event.Change:
		v.Cause = w.stamp(v.Cause)
		w.inner.Emit(v)
	case event.Progress:
		w.inner.Emit(v)
	}
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
