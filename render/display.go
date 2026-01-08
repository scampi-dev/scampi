package render

import (
	"time"

	"godoit.dev/doit/signal"
)

// Message represents a structured, language-agnostic message.
//
// It is intentionally free of prose. Renderers decide how to
// turn this into human-readable output.
type Message struct {
	Key  string
	Args map[string]any

	// Optional extras (ignored for now)
	Hint string
	Help string
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

	UserError(s signal.Severity, msg Message)
	InternalError(s signal.Severity, msg Message)
}

func s(n int) string {
	if n == 1 {
		return ""
	}

	return "s"
}
