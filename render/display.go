package render

import (
	"time"

	"godoit.dev/doit/signal"
)

type Template struct {
	Name string
	Text string
	Hint string
	Help string

	Data any
}

type RunSummary struct {
	ChangedCount int
	FailedCount  int
	TotalCount   int
}

type Displayer interface {
	// Engine lifecycle
	// ===============================================

	EngineStart(s signal.Severity)
	EngineFinish(s signal.Severity, rs RunSummary, dur time.Duration)

	// Planning lifecycle
	// ===============================================

	PlanStart(s signal.Severity)
	UnitPlanned(s signal.Severity, index int, name string, kind string)
	PlanFinish(s signal.Severity, unitCount int, dur time.Duration)
	PlanError(s signal.Severity, index int, name, kind string, tmpl Template)

	// Action lifecycle
	// ===============================================

	ActionStart(s signal.Severity, name string)
	ActionFinish(s signal.Severity, name string, changed bool, dur time.Duration)
	ActionError(s signal.Severity, name string, err error)

	// OpCheck lifecycle
	// ===============================================

	OpCheckStart(s signal.Severity, action string, op string)
	OpCheckSatisfied(s signal.Severity, action string, op string)
	OpCheckUnsatisfied(s signal.Severity, action string, op string)
	OpCheckUnknown(s signal.Severity, action string, op string, err error)

	// OpExecute lifecycle
	// ===============================================

	OpExecuteStart(s signal.Severity, action string, op string)
	OpExecuteFinish(s signal.Severity, action string, op string, changed bool, dur time.Duration)
	OpExecuteError(s signal.Severity, action string, op string, err error)

	// Errors
	// ===============================================

	UserError(s signal.Severity, tmpl Template)
	InternalError(s signal.Severity, tmpl Template)
}

func s(n int) string {
	if n == 1 {
		return ""
	}

	return "s"
}
