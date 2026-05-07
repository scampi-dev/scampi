// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"reflect"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
)

type (
	Policy struct {
		WarningsAsErrors bool
		Verbosity        signal.Verbosity
		SuppressPlan     bool // suppress plan lifecycle events (used by inspect)
		// DedupDiagnostics drops repeat *Diagnostic emissions whose
		// event.Template is structurally identical to one already
		// emitted on the same method. Lifecycle events are never
		// deduped regardless of this flag.
		DedupDiagnostics bool
	}
	policyEmitter struct {
		pol  Policy
		out  Displayer
		seen seenDiags
	}
	// seenDiags records the per-method set of templates already
	// forwarded to the displayer. Each Emit*Diagnostic path keeps
	// its own slice so deduping is scoped to its routing surface
	// (engine vs plan vs action vs op).
	seenDiags struct {
		engine []event.Template
		plan   []event.Template
		action []event.Template
		op     []event.Template
	}
)

func NewEmitter(policy Policy, displayer Displayer) Emitter {
	return &policyEmitter{
		pol: policy,
		out: displayer,
	}
}

// shouldEmit returns true if tmpl is novel for this slot. When dedup
// is enabled and an equal template was previously recorded, returns
// false. Records on success.
func (p *policyEmitter) shouldEmit(slot *[]event.Template, tmpl event.Template) bool {
	if !p.pol.DedupDiagnostics {
		return true
	}
	for _, prior := range *slot {
		if reflect.DeepEqual(prior, tmpl) {
			return false
		}
	}
	*slot = append(*slot, tmpl)
	return true
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

func (p *policyEmitter) EmitGraph(ev event.GraphEvent) {
	ev.Severity = p.pol.apply(ev.Severity)
	p.out.EmitGraph(ev)
}

func (p *policyEmitter) EmitEngineDiagnostic(ev event.EngineDiagnostic) {
	ev.Severity = p.pol.apply(ev.Severity)
	if !p.shouldEmit(&p.seen.engine, ev.Detail.Template) {
		return
	}
	p.out.EmitEngineDiagnostic(ev)
}

func (p *policyEmitter) EmitPlanDiagnostic(ev event.PlanDiagnostic) {
	ev.Severity = p.pol.apply(ev.Severity)
	if !p.shouldEmit(&p.seen.plan, ev.Detail.Template) {
		return
	}
	p.out.EmitPlanDiagnostic(ev)
}

func (p *policyEmitter) EmitActionDiagnostic(ev event.ActionDiagnostic) {
	ev.Severity = p.pol.apply(ev.Severity)
	if !p.shouldEmit(&p.seen.action, ev.Detail.Template) {
		return
	}
	p.out.EmitActionDiagnostic(ev)
}

func (p *policyEmitter) EmitOpDiagnostic(ev event.OpDiagnostic) {
	ev.Severity = p.pol.apply(ev.Severity)
	if !p.shouldEmit(&p.seen.op, ev.Detail.Template) {
		return
	}
	p.out.EmitOpDiagnostic(ev)
}
