// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=OpOutcome
package result

import (
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/spec"
)

// Execution
// -----------------------------------------------------------------------------

// Execution is the aggregated outcome of an apply/check run: one StepReport
// per step across all deploys, plus the terminal error if the run aborted.
type Execution struct {
	Steps []StepReport
	Err   error
}

type StepReport struct {
	Step    spec.Step
	Ops     []OpReport
	Summary event.StepSummary
}

type OpReport struct {
	Op      spec.Op
	Outcome OpOutcome
	Result  *spec.Result
	Err     error
}

type OpOutcome uint8

const (
	OpSucceeded OpOutcome = iota
	OpFailed
	OpAborted
	OpSkipped
	OpWouldChange // Check passed, would execute if applied
)

// Plan
// -----------------------------------------------------------------------------

// Plan is the cross-deploy execution schedule returned by engine.Plan, with
// each deploy's detailed step plan attached as a leaf node. Levels is always
// populated; a single-deploy run is one level with one node and empty
// After/Needs.
type Plan struct {
	Levels []DeployLevel
}

// DeployLevel is one rank in the cross-deploy topology. Nodes within a level
// can run concurrently; every node in level N finishes before level N+1 starts.
type DeployLevel struct {
	Index int
	Nodes []DeployPlan
}

// DeployPlan carries the planned steps for one deploy alongside the edges
// that placed it in its level. After and Needs are nil for roots; the renderer
// uses that to decide whether to draw a cross-deploy graph header.
type DeployPlan struct {
	DeployName string
	TargetName string
	After      []string // deploy names this one waits on
	Needs      []string // resource names that drove After
	Detail     PlanDetail
}

// HasGraph reports whether the plan has cross-deploy ordering worth drawing a
// [graph] section for: at least one deploy waits on another. Independent
// deploys (no edges) add nothing over the per-deploy plan trees, so they don't
// trigger it regardless of how many there are.
func (p Plan) HasGraph() bool {
	for _, level := range p.Levels {
		for _, n := range level.Nodes {
			if len(n.After) > 0 {
				return true
			}
		}
	}
	return false
}

// PlanDetail / PlannedStep / PlannedOp model the rendered structure of
// `scampi plan` output: steps in order, each carrying their ops and
// inter-op dependency edges.
type PlanDetail struct {
	DeployID   string
	TargetName string
	Steps      []PlannedStep
}

type PlannedStep struct {
	Index     int
	Desc      string
	Kind      string
	DependsOn []int
	Ops       []PlannedOp
}

type PlannedOp struct {
	Index     int
	DisplayID string
	DependsOn []int
	Template  *spec.PlanTemplate // nil = no template, use DisplayID
}

// Inspect
// -----------------------------------------------------------------------------

type Inspect struct {
	DeployName string
	TargetName string
	Entries    []InspectEntry
}

type InspectEntry struct {
	Index  int
	Kind   string
	Desc   string
	Fields []spec.InspectField
}
