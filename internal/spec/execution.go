// SPDX-License-Identifier: GPL-3.0-only

package spec

import (
	"context"
	"time"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/controller"
	"scampi.dev/scampi/internal/target"
)

// Plan is the fully planned execution for one resolved Config: a single Deploy
// ready to run.
type Plan struct {
	Deploy Deploy
}

type DeployID string

// Deploy is the execution form of a DeclaredDeploy: a live target plus the
// planned Steps to run against it.
type Deploy struct {
	ID         DeployID
	TargetName string
	Target     target.Target
	Steps      []Step
}

// Step is the execution form of a DeclaredStep: the planned work for one step,
// grouping its Ops into a parallel execution DAG. Produced by StepKind.Plan.
type Step interface {
	Desc() string // optional human description
	Kind() string
	Ops() []Op
}

// StepInputs is an optional interface that steps can implement to declare
// controller-side files they read. The engine pre-caches these so the renderer can
// display source context in error messages.
type StepInputs interface {
	Inputs() []string
}

// Op is the smallest idempotent unit of work. Ops receive both Source
// (host/config state) and Target (system being converged).
type Op interface {
	Step() Step
	Check(ctx context.Context, ctl controller.Controller, tgt target.Target) (CheckResult, []DriftDetail, error)
	Execute(ctx context.Context, ctl controller.Controller, tgt target.Target) (Result, error)
	DependsOn() []Op
	RequiredCapabilities() capability.Capability
}

// OpTimeout is an optional interface that ops can implement to declare a per-op
// timeout. Ops that don't implement it get the default.
type OpTimeout interface {
	Timeout() time.Duration
}

// OpContent is an optional interface that ops producing file content can
// implement to support `scampi inspect --diff`. Both methods take ctl and tgt:
// most ops only need src, but posix.copy with a `source_target { ... }`
// resolver reads desired content from the target itself (#286).
type OpContent interface {
	DesiredContent(ctx context.Context, ctl controller.Controller, tgt target.Target) ([]byte, error)
	CurrentContent(ctx context.Context, ctl controller.Controller, tgt target.Target) ([]byte, error)
	DestPath() string
}

// InspectField is a single label/value pair exposed by OpInspector.
type InspectField struct {
	Label string
	Value string
}

// OpInspection is an optional interface that ops can implement to expose their
// resolved state for `scampi inspect`.
type OpInspection interface {
	Inspect() []InspectField
}

// OpDescription is an optional interface that ops implement to expose their
// plan-line description. The returned OpDescription's Data must be a struct
// so template field references ({{.State}}, {{.Dest}}, ...) are statically
// checkable.
type OpDescription interface {
	Describe() PlanLine
}

// ConfigDedupKey is an optional interface that step configs can implement to
// enable dedup when the same logical step appears multiple times in a steps
// list (e.g. returned from multiple helper functions). Steps with the same
// Kind + DedupKey are collapsed to one; refs to dropped IDs are remapped to the
// survivor.
type ConfigDedupKey interface {
	DedupKey() string
}

// OpOutput is an optional interface that ops can implement to expose their
// settled state after execution. The engine captures this for ref() resolution
// in downstream steps.
type OpOutput interface {
	Output() any
}

// PlanLine is the render contract for an op's plan line.
type PlanLine struct {
	ID   string
	Text string
	Data any
}

func (t PlanLine) TemplateID() string   { return t.ID }
func (t PlanLine) TemplateText() string { return t.Text }
func (t PlanLine) TemplateData() any    { return t.Data }

type Result struct {
	Changed bool
}

//go:generate stringer -type=CheckResult
type CheckResult uint8

const (
	CheckUnknown CheckResult = iota
	CheckSatisfied
	CheckUnsatisfied
)
