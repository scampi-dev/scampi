//go:generate stringer -type=CheckResult
package spec

import (
	"context"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/source"
	"godoit.dev/doit/target"
)

type (
	Config struct {
		Unit    UnitInstance
		Steps   []StepInstance
		Sources SourceStore
	}
	UnitInstance struct {
		ID   UnitID
		Desc string
	}
	StepInstance struct {
		Desc   string // optional human description
		Type   StepType
		Config any
		Source SourceSpan
		Fields map[string]FieldSpan
	}
	StepType interface {
		Kind() string
		// NewConfig MUST return a pointer to a freshly allocated config struct.
		// Returning a value will cause undefined behavior.
		NewConfig() any
		Plan(idx int, step StepInstance) (Action, error)
	}
	FieldSpan struct {
		Field SourceSpan
		Value SourceSpan
	}
	SourceSpan struct {
		Filename  string
		StartLine int
		EndLine   int
		StartCol  int
		EndCol    int
	}

	Plan struct {
		Unit Unit
	}
	UnitID string
	Unit   struct {
		ID      UnitID
		Desc    string
		Actions []Action
	}
	Action interface {
		Desc() string // optional human description
		Kind() string
		Ops() []Op
	}
	Op interface {
		Action() Action
		Check(ctx context.Context, src source.Source, tgt target.Target) (CheckResult, error)
		Execute(ctx context.Context, src source.Source, tgt target.Target) (Result, error)
		DependsOn() []Op
		RequiredCapabilities() capability.Capability
	}
	OpDescriber interface {
		OpDescription() OpDescription
	}
	OpDescription interface {
		PlanTemplate() PlanTemplate
	}

	PlanTemplate struct {
		ID   string
		Text string
		Data any
	}
	Result struct {
		Changed bool
	}
)

type CheckResult uint8

const (
	CheckUnknown CheckResult = iota
	CheckSatisfied
	CheckUnsatisfied
)
