// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"context"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

type (
	CheckFn func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error)
	ExecFn  func(context.Context, source.Source, target.Target) (spec.Result, error)
	FakeOp  struct {
		Name       string
		action     spec.Action
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

func (o FakeOp) Action() spec.Action  { return o.action }
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

type FakeAction struct {
	ops []spec.Op
}

// AddOp appends an op to this action (used by tests that can't use MkAction).
func (a *FakeAction) AddOp(op spec.Op) { a.ops = append(a.ops, op) }

func (FakeAction) Kind() string     { return "fakeActionKind" }
func (FakeAction) Desc() string     { return "fakeAction" }
func (a FakeAction) Ops() []spec.Op { return a.ops }

// SetAction sets the parent action for this op (used by tests that
// can't use MkAction for custom action types).
func (o *FakeOp) SetAction(a spec.Action) { o.action = a }

func MkAction(ops ...*FakeOp) *FakeAction {
	act := &FakeAction{}

	for _, op := range ops {
		act.ops = append(act.ops, op)
		op.action = act
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

func (d FakeDiagnostic) EventTemplate() event.Template {
	return event.Template{
		ID:   "test.FakeDiagnostic",
		Text: "{{if .}}{{.}}{{else}}test diagnostic{{end}}",
		Data: d.cause,
	}
}

func (d FakeDiagnostic) Severity() signal.Severity { return d.severity }
func (d FakeDiagnostic) Impact() diagnostic.Impact { return d.impact }

// StubStepType returns a pre-built action from Plan, bypassing real config
// parsing. Useful when tests need to control exactly which ops are planned.
type StubStepType struct {
	StepKind   string
	StepAction spec.Action
}

func (s *StubStepType) Kind() string   { return s.StepKind }
func (s *StubStepType) NewConfig() any { return &struct{}{} }
func (s *StubStepType) Plan(_ spec.StepInstance) (spec.Action, error) {
	return s.StepAction, nil
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
