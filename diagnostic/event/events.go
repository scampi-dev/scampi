package event

import (
	"time"

	"godoit.dev/doit/model"
	"godoit.dev/doit/spec"
)

type EngineDetail struct {
	ChangedCount int
	FailedCount  int
	TotalCount   int
	Duration     time.Duration
	Err          error
}

type PlanFinishedDetail struct {
	SuccessfulUnits int
	FailedUnits     int
	Duration        time.Duration
}
type PlanDetail struct {
	Actions []PlannedAction
}
type PlannedAction struct {
	Index int
	Name  string
	Kind  string
	Ops   []PlannedOp
}
type PlannedOp struct {
	Index     int
	Name      string
	DependsOn []int // DAG edges (indices of other PlannedOps)
	Template  *spec.PlanTemplate
}
type PlanProblem struct {
	Index int
	Name  string
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

type DiagnosticDetail struct {
	Template Template
}
