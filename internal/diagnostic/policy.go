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

type policyEmitter struct {
	pol Policy
	out Output

	// mu makes the emitter the single serialization point: the engine emits
	// from parallel op/action goroutines, but RenderEvent is delivered one
	// call at a time. This is the whole contract — Output implementations are
	// therefore single-threaded and carry no locks of their own.
	mu sync.Mutex
}

// NewEmitter returns an emitter that forwards events to the output backend,
// applying the policy's severity remap on the way. Emit/Raise are safe for
// concurrent use; delivery to the Output is serialized.
func NewEmitter(policy Policy, out Output) Emitter {
	return &policyEmitter{pol: policy, out: out}
}

func (p *policyEmitter) Raise(err Raisable) {
	p.Emit(err.Diagnostic())
}

func (p *policyEmitter) Emit(ev event.Event) {
	// WarningsAsErrors flips the type but not Impact: the producer
	// decides whether a diagnostic aborts, not the policy.
	if p.pol.WarningsAsErrors {
		if w, ok := ev.(event.Warning); ok {
			ev = event.Error{
				Time:     w.Time,
				Template: w.Template,
				Cause:    w.Cause,
			}
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.out.RenderEvent(ev)
}
