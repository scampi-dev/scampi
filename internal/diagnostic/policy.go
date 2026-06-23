// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
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
	out Displayer
}

// NewEmitter returns a stateless emitter that forwards events to
// displayer, applying the policy's severity remap on the way.
func NewEmitter(policy Policy, displayer Displayer) Emitter {
	return &policyEmitter{pol: policy, out: displayer}
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
	p.out.Emit(ev)
}
