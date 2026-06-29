// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"sync"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/signal"
)

// Policy is consumed by NewEmitter to shape how diagnostics surface.
// WarningsAsErrors promotes warning-severity diagnostics to errors.
// Verbosity is forwarded to render consumers; it does not gate the
// emit pipeline itself.
type Policy struct {
	WarningsAsErrors bool
	Verbosity        signal.Verbosity
}

// Emitter is the producer side of the diagnostic pipeline and the system's
// single serialization point: the engine emits from parallel op/step
// goroutines, but the mutex delivers events to the Output one call at a time.
// There is exactly one Emitter type, and because it serializes, every Output
// is single-threaded and lock-free.
//
// Almost nothing holds an Emitter directly: it is created at the command
// boundary, handed to NewCtx, and from then on threaded as a diagnostic.Ctx.
// Always used by pointer (it holds a mutex).
type Emitter struct {
	pol Policy
	out Output
	mu  sync.Mutex
}

// NewEmitter returns an emitter that forwards events to the output backend,
// applying the policy's severity remap on the way. Emit/Raise are safe for
// concurrent use; delivery to the Output is serialized.
func NewEmitter(policy Policy, out Output) *Emitter {
	return &Emitter{pol: policy, out: out}
}

func (e *Emitter) Raise(err Raisable) {
	e.Emit(err.Diagnostic())
}

func (e *Emitter) Emit(ev event.Event) {
	// WarningsAsErrors flips the type but not Impact: the producer
	// decides whether a diagnostic aborts, not the policy.
	if e.pol.WarningsAsErrors {
		if w, ok := ev.(event.Warning); ok {
			ev = event.Error{
				Time:     w.Time,
				Template: w.Template,
				Cause:    w.Cause,
			}
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.out.RenderEvent(ev)
}
