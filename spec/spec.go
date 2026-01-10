package spec

import (
	"context"

	"godoit.dev/doit/target"
)

type (
	Config struct {
		Units []UnitInstance
	}
	UnitInstance struct {
		Name   string
		Type   UnitType
		Config any
		Source SourceSpan
		Fields map[string]SourceSpan
	}
	UnitType interface {
		Kind() string
		NewConfig() any
		Plan(idx int, unit UnitInstance) (Action, error)
	}
	SourceSpan struct {
		Filename string
		Line     int
		Column   int
	}

	Plan struct {
		Actions []Action
	}
	Action interface {
		Name() string
		Ops() []Op
	}
	Op interface {
		Name() string
		Action() string
		Check(ctx context.Context, tgt target.Target) (CheckResult, error)
		Execute(ctx context.Context, tgt target.Target) (Result, error)
		DependsOn() []Op
	}
	Result struct {
		Changed bool
	}
	CheckResult uint8
)

const (
	CheckUnknown CheckResult = iota
	CheckSatisfied
	CheckUnsatisfied
)
