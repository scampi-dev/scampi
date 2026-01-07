package render

import (
	"time"

	"godoit.dev/doit/signal"
)

type Displayer interface {
	// Engine lifecycle
	// =============================================

	EngineStart(s signal.Severity)
	EngineFinish(s signal.Severity, changed, ttl int, duration time.Duration)

	// Config / planning phase
	// =============================================

	PlanStart(s signal.Severity)
	UnitPlanned(s signal.Severity, index int, name string, kind string)
	PlanFinish(s signal.Severity, unitCount int, duration time.Duration)

	// Action lifecycle
	// =============================================

	ActionStart(s signal.Severity, name string)
	ActionFinish(s signal.Severity, name string, changed bool, duration time.Duration)
	ActionError(s signal.Severity, name string, err error)

	// Ops signals
	// =============================================

	OpCheckStart(s signal.Severity, action string, op string)
	OpCheckSatisfied(s signal.Severity, action string, op string)
	OpCheckUnsatisfied(s signal.Severity, action string, op string)
	OpCheckUnknown(s signal.Severity, action string, op string, err error)

	OpExecuteStart(s signal.Severity, action string, op string)
	OpExecuteFinish(s signal.Severity, action string, op string, changed bool, duration time.Duration)
	OpExecuteError(s signal.Severity, action string, op string, err error)

	// User-visible errors (s signal.Severity,expected, actionable)
	// =============================================

	UserError(s signal.Severity, message string)

	// Internal errors (s signal.Severity,bugs, invariants violated)
	// =============================================

	InternalError(s signal.Severity, message string, err error)
}

func pluralS(n int, singular string) string {
	if n == 1 {
		return singular
	}

	return singular + "s"
}
