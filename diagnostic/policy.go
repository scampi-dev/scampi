// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
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

func (p *policyEmitter) EmitDiagnostic(ev event.Diagnostic) {
	if p.pol.WarningsAsErrors && ev.Severity == signal.Warning {
		ev.Severity = signal.Error
	}
	p.out.EmitDiagnostic(ev)
}

func (p *policyEmitter) EmitChange(ev event.Change) {
	p.out.EmitChange(ev)
}

func (p *policyEmitter) EmitProgress(ev event.Progress) {
	p.out.EmitProgress(ev)
}
