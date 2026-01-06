package spec

import (
	"context"

	"godoit.dev/doit/target"
)

type (
	Config struct {
		Tasks []CfgTask
	}
	CfgTask struct {
		Kind   string
		Name   string
		Spec   Spec
		Config any
	}

	Spec interface {
		Kind() string
		NewConfig() any
		Plan(idx int, cfg any) (RtTask, error)
	}
	RtPlan struct {
		Tasks []RtTask
	}
	RtTask interface {
		Name() string
		Ops() []Op
	}
	Op interface {
		Name() string
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
