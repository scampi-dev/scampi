package diagnostic

import (
	"time"
)

type (
	RunSummary struct {
		ChangedCount int
		FailedCount  int
		TotalCount   int
	}
	Template struct {
		Name string
		Text string
		Hint string
		Help string
	}
)

type Diagnostic interface {
	Template() Template
}

type Emitter interface {
	// Engine lifecycle
	// ===============================================

	EngineStart()
	EngineFinish(rs RunSummary, dur time.Duration)

	// Planning lifecycle
	// ===============================================

	PlanStart()
	UnitPlanned(index int, name string, kind string)
	PlanFinish(unitCount int, dur time.Duration)
	PlanError(index int, name string, kind string, diag Diagnostic)

	// Action lifecycle
	// ===============================================

	ActionStart(name string)
	ActionFinish(name string, changed bool, dur time.Duration)
	ActionError(name string, err error)

	// OpCheck lifecycle
	// ===============================================

	OpCheckStart(action string, op string)
	OpCheckSatisfied(action string, op string)
	OpCheckUnsatisfied(action string, op string)
	OpCheckUnknown(action string, op string, err error)

	// OpExecute lifecycle
	// ===============================================

	OpExecuteStart(action string, op string)
	OpExecuteFinish(action string, op string, changed bool, dur time.Duration)
	OpExecuteError(action string, op string, err error)

	// Errors
	// ===============================================

	UserError(diag Diagnostic)
	InternalError(message string, err error)
}
