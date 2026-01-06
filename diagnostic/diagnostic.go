package diagnostic

import (
	"time"
)

type Emitter interface {
	// Engine lifecycle
	// =============================================

	EngineStart()
	EngineFinish(changed, ttl int, duration time.Duration)

	// Config / planning phase
	// =============================================

	PlanStart()
	UnitPlanned(index int, name string, kind string)
	PlanFinish(unitCount int, duration time.Duration)

	// Action lifecycle
	// =============================================

	ActionStart(name string)
	ActionFinish(name string, changed bool, duration time.Duration)
	ActionError(name string, err error)

	// Ops diagnostics
	// =============================================

	OpCheckStart(action string, op string)
	OpCheckSatisfied(action string, op string)
	OpCheckUnsatisfied(action string, op string)
	OpCheckUnknown(action string, op string, err error)

	OpExecuteStart(action string, op string)
	OpExecuteFinish(action string, op string, changed bool, duration time.Duration)
	OpExecuteError(action string, op string, err error)

	// User-visible errors (expected, actionable)
	// =============================================

	UserError(message string)

	// Internal errors (bugs, invariants violated)
	// =============================================

	InternalError(message string, err error)
}
