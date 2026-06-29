// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"context"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

type (
	CheckFn func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error)
	ExecFn  func(context.Context, source.Source, target.Target) (spec.Result, error)
	FakeOp  struct {
		Name       string
		step       spec.Step
		Deps       []spec.Op
		CheckFn    CheckFn
		ExecFn     ExecFn
		CheckCalls int
		ExecCalls  int
	}
)

func OkCheckFn(res spec.CheckResult) CheckFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		return res, nil, nil
	}
}

func OkExecFn(changed bool) ExecFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		return spec.Result{Changed: changed}, nil
	}
}

func DiagCheckFn(severity signal.Severity, impact diagnostic.Impact) CheckFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		return spec.CheckUnknown, nil, &FakeDiagnostic{
			severity: severity,
			impact:   impact,
		}
	}
}

func DiagExecFn(severity signal.Severity, impact diagnostic.Impact) ExecFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		return spec.Result{}, &FakeDiagnostic{
			severity: severity,
			impact:   impact,
		}
	}
}

//lint:ignore U1000
func ErrCheckFn(err error) CheckFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		return spec.CheckUnknown, nil, err
	}
}

//lint:ignore U1000
func ErrExecFn(err error) ExecFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		return spec.Result{}, err
	}
}

func PanicCheckFn(msg string) CheckFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		panic(msg)
	}
}

func PanicExecFn(msg string) ExecFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		panic(msg)
	}
}

func (o FakeOp) Step() spec.Step      { return o.step }
func (o FakeOp) DependsOn() []spec.Op { return o.Deps }

func (o *FakeOp) Check(
	ctx context.Context,
	src source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	o.CheckCalls++
	return o.CheckFn(ctx, src, tgt)
}

func (o *FakeOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	o.ExecCalls++
	return o.ExecFn(ctx, src, tgt)
}

func (o FakeOp) OpDescription() spec.OpDescription {
	return o
}

func (o FakeOp) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{ID: o.Name}
}

func (FakeOp) RequiredCapabilities() capability.Capability {
	return capability.POSIX
}

type FakeStep struct {
	ops []spec.Op
}

// AddOp appends an op to this step (used by tests that can't use MkStep).
func (a *FakeStep) AddOp(op spec.Op) { a.ops = append(a.ops, op) }

func (FakeStep) Kind() string     { return "fakeStepKind" }
func (FakeStep) Desc() string     { return "fakeStep" }
func (a FakeStep) Ops() []spec.Op { return a.ops }

// SetStep sets the parent step for this op (used by tests that
// can't use MkStep for custom step types).
func (o *FakeOp) SetStep(a spec.Step) { o.step = a }

func MkStep(ops ...*FakeOp) *FakeStep {
	act := &FakeStep{}

	for _, op := range ops {
		act.ops = append(act.ops, op)
		op.step = act
	}

	return act
}

type FakeDiagnostic struct {
	severity signal.Severity
	impact   diagnostic.Impact
	cause    error
}

func NewFakeDiagnostic(severity signal.Severity, impact diagnostic.Impact, cause error) *FakeDiagnostic {
	return &FakeDiagnostic{severity: severity, impact: impact, cause: cause}
}

func (d FakeDiagnostic) Error() string {
	if d.cause != nil {
		return d.cause.Error()
	}
	return "fake diagnostic"
}

func (d FakeDiagnostic) Unwrap() error { return d.cause }

func (d FakeDiagnostic) Diagnostic() event.Event {
	tmpl := event.Template{
		ID:   "test.FakeDiagnostic",
		Text: "{{if .}}{{.}}{{else}}test diagnostic{{end}}",
		Data: d.cause,
	}
	switch d.severity {
	case signal.Warning:
		return event.Warning{Template: tmpl}
	case signal.Info:
		return event.Info{Template: tmpl}
	default:
		impact := event.ImpactNone
		if d.impact == diagnostic.ImpactAbort {
			impact = event.ImpactAbort
		}
		return event.Error{Impact: impact, Template: tmpl}
	}
}

// StubStepKind returns a pre-built step from Plan, bypassing real config
// parsing. Useful when tests need to control exactly which ops are planned.
type StubStepKind struct {
	StepKind string
	StepStep spec.Step
}

func (s *StubStepKind) Kind() string   { return s.StepKind }
func (s *StubStepKind) NewConfig() any { return &struct{}{} }
func (s *StubStepKind) Plan(_ spec.DeclaredStep) (spec.Step, error) {
	return s.StepStep, nil
}

// InspectableFakeOp is a FakeOp that also implements spec.Diffable.
type InspectableFakeOp struct {
	FakeOp
	Desired []byte
	Current []byte
	CurrErr error
	Dest    string
}

func (o *InspectableFakeOp) DesiredContent(_ context.Context, _ source.Source, _ target.Target) ([]byte, error) {
	return o.Desired, nil
}

func (o *InspectableFakeOp) CurrentContent(_ context.Context, _ source.Source, _ target.Target) ([]byte, error) {
	return o.Current, o.CurrErr
}

func (o *InspectableFakeOp) DestPath() string {
	return o.Dest
}
