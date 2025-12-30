package spec

import "context"

type (
	Config struct {
		Tasks []CfgTask
	}
	CfgTask struct {
		Kind   string
		Spec   Spec
		Config any
	}

	Spec interface {
		Kind() string
		Schema() string
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
		Check(ctx context.Context) (CheckResult, error)
		Execute(ctx context.Context) (Result, error)
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
