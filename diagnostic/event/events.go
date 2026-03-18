// SPDX-License-Identifier: GPL-3.0-only

package event

import (
	"time"

	"scampi.dev/scampi/model"
	"scampi.dev/scampi/spec"
)

type EngineFinishedDetail struct {
	CheckOnly        bool // true for check command, false for apply
	Cancelled        bool // true when interrupted by signal
	ChangedCount     int
	WouldChangeCount int
	FailedCount      int
	TotalCount       int
	HooksFired       int
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
	Index     int
	Desc      string
	Kind      string
	DependsOn []int // indices of actions this depends on
	Ops       []PlannedOp
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

type HookDetail struct {
	HookID    string
	TriggerBy string // desc of the step/hook that triggered this hook (empty for skipped)
	Summary   model.ActionSummary
	Duration  time.Duration
}

type OpCheckDetail struct {
	Result spec.CheckResult
	Err    error
	Drift  []spec.DriftDetail
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
