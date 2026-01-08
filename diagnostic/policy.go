package diagnostic

import (
	"time"

	"godoit.dev/doit/render"
	"godoit.dev/doit/signal"
)

type (
	Policy struct {
		WarningsAsErrors bool
		Verbosity        signal.Verbosity
	}
	Decision struct {
		Severity signal.Severity
		Show     bool
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

// Engine lifecycle
// ===============================================

func (e *policyEmitter) EngineStart() {
	if e.pol.Verbosity >= signal.VV {
		e.out.EngineStart(e.pol.apply(signal.Info))
	}
}

func (e *policyEmitter) EngineFinish(rs RunSummary, duration time.Duration) {
	drs := render.RunSummary{
		ChangedCount: rs.ChangedCount,
		FailedCount:  rs.FailedCount,
		TotalCount:   rs.TotalCount,
	}
	e.out.EngineFinish(e.pol.apply(signal.Important), drs, duration)
}

// Planning lifecycle
// ===============================================

func (e *policyEmitter) PlanStart() {
	if e.pol.Verbosity >= signal.VV {
		e.out.PlanStart(e.pol.apply(signal.Info))
	}
}

func (e *policyEmitter) UnitPlanned(index int, name, kind string) {
	if e.pol.Verbosity >= signal.VV {
		e.out.UnitPlanned(e.pol.apply(signal.Debug), index, name, kind)
	}
}

func (e *policyEmitter) PlanFinish(unitCount int, duration time.Duration) {
	if e.pol.Verbosity >= signal.VV {
		e.out.PlanFinish(e.pol.apply(signal.Info), unitCount, duration)
	}
}

// Action lifecycle
// ===============================================

func (e *policyEmitter) ActionStart(name string) {
	if e.pol.Verbosity >= signal.V {
		e.out.ActionStart(e.pol.apply(signal.Notice), name)
	}
}

func (e *policyEmitter) ActionFinish(name string, changed bool, duration time.Duration) {
	if changed {
		e.out.ActionFinish(e.pol.apply(signal.Important), name, changed, duration)
		return
	}

	if e.pol.Verbosity >= signal.V {
		e.out.ActionFinish(e.pol.apply(signal.Info), name, changed, duration)
	}
}

func (e *policyEmitter) ActionError(name string, err error) {
	e.out.ActionError(e.pol.apply(signal.Error), name, err)
}

// OpCheck lifecycle
// ===============================================

func (e *policyEmitter) OpCheckStart(action, op string) {
	if e.pol.Verbosity >= signal.VV {
		e.out.OpCheckStart(e.pol.apply(signal.Debug), action, op)
	}
}

func (e *policyEmitter) OpCheckSatisfied(action, op string) {
	if e.pol.Verbosity >= signal.VV {
		e.out.OpCheckSatisfied(e.pol.apply(signal.Debug), action, op)
	}
}

func (e *policyEmitter) OpCheckUnsatisfied(action, op string) {
	if e.pol.Verbosity >= signal.V {
		e.out.OpCheckUnsatisfied(e.pol.apply(signal.Notice), action, op)
	}
}

func (e *policyEmitter) OpCheckUnknown(action, op string, err error) {
	e.out.OpCheckUnknown(e.pol.apply(signal.Warning), action, op, err)
}

// OpExecute lifecycle
// ===============================================

func (e *policyEmitter) OpExecuteStart(action, op string) {
	if e.pol.Verbosity >= signal.VV {
		e.out.OpExecuteStart(e.pol.apply(signal.Debug), action, op)
	}
}

func (e *policyEmitter) OpExecuteFinish(action, op string, changed bool, duration time.Duration) {
	if changed {
		if e.pol.Verbosity >= signal.VV {
			e.out.OpExecuteFinish(e.pol.apply(signal.Info), action, op, changed, duration)
		}
		return
	}

	if e.pol.Verbosity >= signal.VV {
		e.out.OpExecuteFinish(e.pol.apply(signal.Debug), action, op, changed, duration)
	}
}

func (e *policyEmitter) OpExecuteError(action, op string, err error) {
	e.out.OpExecuteError(e.pol.apply(signal.Error), action, op, err)
}

// Errors
// ===============================================

func (e *policyEmitter) UserError(message string) {
	e.out.UserError(
		e.pol.apply(signal.Error),
		render.Message{
			Key: "legacy.message",
			Args: map[string]any{
				"msg": message,
			},
		},
	)
}

func (e *policyEmitter) InternalError(message string, err error) {
	e.out.InternalError(
		e.pol.apply(signal.Error),
		render.Message{
			Key: "legacy.message",
			Args: map[string]any{
				"msg": message,
				"err": err,
			},
		},
	)
}
