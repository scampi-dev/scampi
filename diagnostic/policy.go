// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/render"
	"scampi.dev/scampi/signal"
)

type (
	Policy struct {
		WarningsAsErrors bool
		Verbosity        signal.Verbosity
		SuppressPlan     bool // suppress plan lifecycle events (used by inspect)
	}
	policyEmitter struct {
		pol Policy
		out render.Displayer
	}
)

func NewEmitter(policy Policy, displayer render.Displayer) Emitter {
	return &policyEmitter{
		pol: policy,
		out: displayer,
	}
}

func (p Policy) apply(s signal.Severity) signal.Severity {
	if s == signal.Warning && p.WarningsAsErrors {
		return signal.Error
	}

	return s
}

func (p *policyEmitter) EmitEngineLifecycle(ev event.EngineEvent) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitEngineLifecycle(ev)
}

func (p *policyEmitter) EmitPlanLifecycle(ev event.PlanEvent) {
	if p.pol.SuppressPlan {
		return
	}
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitPlanLifecycle(ev)
}

func (p *policyEmitter) EmitActionLifecycle(ev event.ActionEvent) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitActionLifecycle(ev)
}

func (p *policyEmitter) EmitOpLifecycle(ev event.OpEvent) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitOpLifecycle(ev)
}

func (p *policyEmitter) EmitIndexAll(ev event.IndexAllEvent) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitIndexAll(ev)
}

func (p *policyEmitter) EmitIndexStep(ev event.IndexStepEvent) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitIndexStep(ev)
}

func (p *policyEmitter) EmitInspect(ev event.InspectEvent) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitInspect(ev)
}

func (p *policyEmitter) EmitEngineDiagnostic(ev event.EngineDiagnostic) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitEngineDiagnostic(ev)
}

func (p *policyEmitter) EmitPlanDiagnostic(ev event.PlanDiagnostic) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitPlanDiagnostic(ev)
}

func (p *policyEmitter) EmitActionDiagnostic(ev event.ActionDiagnostic) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitActionDiagnostic(ev)
}

func (p *policyEmitter) EmitOpDiagnostic(ev event.OpDiagnostic) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitOpDiagnostic(ev)
}
