// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"reflect"
	"sync"

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
		mu   sync.Mutex // guards seen — diagnostics emit from op/plan goroutines (#329)
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
// false. Records on success. Safe for concurrent callers — the emit
// pipeline is hit from op pool, plan workers, and engine goroutines.
func (p *policyEmitter) shouldEmit(slot *[]event.Template, tmpl event.Template) bool {
	if !p.pol.DedupDiagnostics {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
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

// New diagnostic surface - see doc/design/diagnostics.md.
// -----------------------------------------------------------------------------
// These methods are wired through but inert in phase 1 - the CLI
// renderer is rebuilt in phase 3. Until then producers emitting via
// the v2 methods are visible to test displayers but not to the user.

func (p *policyEmitter) EmitDiagnostic(ev event.Diagnostic) {
	p.out.EmitDiagnostic(ev)
}

func (p *policyEmitter) EmitChange(ev event.Change) {
	p.out.EmitChange(ev)
}

func (p *policyEmitter) EmitProgress(ev event.Progress) {
	p.out.EmitProgress(ev)
}
