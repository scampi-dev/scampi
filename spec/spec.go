package spec

import (
	"context"

	"godoit.dev/doit/source"
	"godoit.dev/doit/target"
)

type (
	Config struct {
		Units   []UnitInstance
		Sources SourceStore
	}
	UnitInstance struct {
		Name   string
		Type   UnitType
		Config any
		Source SourceSpan
		Fields map[string]FieldSpan
	}
	UnitType interface {
		Kind() string
		// NewConfig MUST return a pointer to a freshly allocated config struct.
		// Returning a value will cause undefined behavior.
		NewConfig() any
		Plan(idx int, unit UnitInstance) (Action, error)
	}
	FieldSpan struct {
		Field SourceSpan
		Value SourceSpan
	}
	SourceSpan struct {
		Filename string
		Line     int
		StartCol int
		EndCol   int
	}

	Plan struct {
		Actions []Action
	}
	Action interface {
		Name() string
		Kind() string
		Ops() []Op
	}
	Op interface {
		Name() string
		Action() Action
		Check(ctx context.Context, src source.Source, tgt target.Target) (CheckResult, error)
		Execute(ctx context.Context, src source.Source, tgt target.Target) (Result, error)
		DependsOn() []Op
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
