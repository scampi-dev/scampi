package event

import (
	"time"

	"godoit.dev/doit/model"
	"godoit.dev/doit/spec"
)

type EngineFinishedDetail struct {
	CheckOnly        bool // true for check command, false for apply
	ChangedCount     int
	WouldChangeCount int
	FailedCount      int
	TotalCount       int
	Duration         time.Duration
	Err              error
}

type PlanStartedDetail struct {
	UnitID string
}
type PlanFinishedDetail struct {
	UnitID          string
	SuccessfulSteps int
	FailedSteps     int
	Duration        time.Duration
}
type PlanDetail struct {
	UnitID   string
	UnitDesc string
	Actions  []PlannedAction
}
type PlannedAction struct {
	Index int
	Desc  string
	Kind  string
	Ops   []PlannedOp
}
type PlannedOp struct {
	Index     int
	DisplayID string // derived from OpDescriber or fallback
	DependsOn []int  // DAG edges (indices of other PlannedOps)
	Template  *spec.PlanTemplate
}
type PlanProblem struct {
	Index int
	Desc  string
	Kind  string
	Err   error
}

type ActionDetail struct {
	Summary  model.ActionSummary
	Duration time.Duration
	Err      error
}

type OpCheckDetail struct {
	Result spec.CheckResult
	Err    error
}

type OpExecuteDetail struct {
	Changed  bool
	Duration time.Duration
	Err      error
}

type StepIndexDetail struct {
	Kind string
	Desc string
}

type DiagnosticDetail struct {
	Template Template
}
