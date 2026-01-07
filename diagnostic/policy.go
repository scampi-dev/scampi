package diagnostic

import (
	"time"

	"godoit.dev/doit/render"
	"godoit.dev/doit/signal"
)

type (
	Policy struct {
		WarningsAsErrors bool
		Verbosity        Verbosity
	}
	Decision struct {
		Severity signal.Severity
		Show     bool
	}
	policyEmitter struct {
		pol Policy
		dsp render.Displayer
	}
)

func NewEmitter(policy Policy, displayer render.Displayer) Emitter {
	return &policyEmitter{
		pol: policy,
		dsp: displayer,
	}
}

func (p Policy) Apply(s signal.Severity) Decision {
	if s == signal.Warning && p.WarningsAsErrors {
		s = signal.Error
	}

	return Decision{Severity: s, Show: p.shouldShow(s)}
}

func (p Policy) shouldShow(s signal.Severity) bool {
	switch s {
	case signal.Fatal, signal.Error, signal.Warning:
		return true

	case signal.Important:
		return p.Verbosity >= Quiet

	case signal.Notice:
		return p.Verbosity >= Verbose

	case signal.Info:
		return p.Verbosity >= VeryVerbose

	case signal.Debug:
		return p.Verbosity >= DebugVerbose

	default:
		return false
	}
}

// Engine lifecycle
// =============================================

func (e *policyEmitter) EngineStart() {
	d := e.pol.Apply(signal.Info)
	if d.Show {
		e.dsp.EngineStart(d.Severity)
	}
}

func (e *policyEmitter) EngineFinish(nChanged, nUnits int, duration time.Duration) {
	d := e.pol.Apply(signal.Important)
	if d.Show {
		e.dsp.EngineFinish(d.Severity, nChanged, nUnits, duration)
	}
}

// Config / planning phase
// =============================================

func (e *policyEmitter) PlanStart() {
	d := e.pol.Apply(signal.Info)
	if d.Show {
		e.dsp.PlanStart(d.Severity)
	}
}

func (e *policyEmitter) UnitPlanned(index int, name, kind string) {
	d := e.pol.Apply(signal.Debug)
	if d.Show {
		e.dsp.UnitPlanned(d.Severity, index, name, kind)
	}
}

func (e *policyEmitter) PlanFinish(unitCount int, duration time.Duration) {
	d := e.pol.Apply(signal.Info)
	if d.Show {
		e.dsp.PlanFinish(d.Severity, unitCount, duration)
	}
}

// Action lifecycle
// =============================================

func (e *policyEmitter) ActionStart(name string) {
	d := e.pol.Apply(signal.Notice)
	if d.Show {
		e.dsp.ActionStart(d.Severity, name)
	}
}

func (e *policyEmitter) ActionFinish(name string, changed bool, duration time.Duration) {
	var s signal.Severity
	if changed {
		s = signal.Important
	} else {
		s = signal.Notice
	}

	d := e.pol.Apply(s)
	if d.Show {
		e.dsp.ActionFinish(d.Severity, name, changed, duration)
	}
}

func (e *policyEmitter) ActionError(name string, err error) {
	d := e.pol.Apply(signal.Error)
	if d.Show {
		e.dsp.ActionError(d.Severity, name, err)
	}
}

// Ops diagnostics
// =============================================

func (e *policyEmitter) OpCheckStart(action, op string) {
	d := e.pol.Apply(signal.Debug)
	if d.Show {
		e.dsp.OpCheckStart(d.Severity, action, op)
	}
}

func (e *policyEmitter) OpCheckSatisfied(action, op string) {
	d := e.pol.Apply(signal.Debug)
	if d.Show {
		e.dsp.OpCheckSatisfied(d.Severity, action, op)
	}
}

func (e *policyEmitter) OpCheckUnsatisfied(action, op string) {
	d := e.pol.Apply(signal.Notice)
	if d.Show {
		e.dsp.OpCheckUnsatisfied(d.Severity, action, op)
	}
}

func (e *policyEmitter) OpCheckUnknown(action, op string, err error) {
	d := e.pol.Apply(signal.Warning)
	if d.Show {
		e.dsp.OpCheckUnknown(d.Severity, action, op, err)
	}
}

func (e *policyEmitter) OpExecuteStart(action, op string) {
	d := e.pol.Apply(signal.Debug)
	if d.Show {
		e.dsp.OpExecuteStart(d.Severity, action, op)
	}
}

func (e *policyEmitter) OpExecuteFinish(action, op string, changed bool, duration time.Duration) {
	d := e.pol.Apply(signal.Info)
	if d.Show {
		e.dsp.OpExecuteFinish(d.Severity, action, op, changed, duration)
	}
}

func (e *policyEmitter) OpExecuteError(action, op string, err error) {
	d := e.pol.Apply(signal.Error)
	if d.Show {
		e.dsp.OpExecuteError(d.Severity, action, op, err)
	}
}

// User-visible errors (expected, actionable)
// =============================================

func (e *policyEmitter) UserError(message string) {
	d := e.pol.Apply(signal.Error)
	if d.Show {
		e.dsp.UserError(d.Severity, message)
	}
}

// Internal errors (bugs, invariants violated)
// =============================================

func (e *policyEmitter) InternalError(message string, err error) {
	d := e.pol.Apply(signal.Fatal)
	if d.Show {
		e.dsp.InternalError(d.Severity, message, err)
	}
}
