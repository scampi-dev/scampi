// SPDX-License-Identifier: GPL-3.0-only

package test

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
	checkFn func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error)
	execFn  func(context.Context, source.Source, target.Target) (spec.Result, error)
	fakeOp  struct {
		name   string
		action spec.Action
		deps   []spec.Op

		checkFn checkFn
		execFn  execFn

		checkCalls int
		execCalls  int
	}
)

func okCheckFn(res spec.CheckResult) checkFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		return res, nil, nil
	}
}

func okExecFn(changed bool) execFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		return spec.Result{Changed: changed}, nil
	}
}

func diagCheckFn(severity signal.Severity, impact diagnostic.Impact) checkFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		return spec.CheckUnknown, nil, &fakeDiagnostic{
			severity: severity,
			impact:   impact,
		}
	}
}

func diagExecFn(severity signal.Severity, impact diagnostic.Impact) execFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		return spec.Result{}, &fakeDiagnostic{
			severity: severity,
			impact:   impact,
		}
	}
}

//lint:ignore U1000
func errCheckFn(err error) checkFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		return spec.CheckUnknown, nil, err
	}
}

//lint:ignore U1000
func errExecFn(err error) execFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		return spec.Result{}, err
	}
}

func panicCheckFn(msg string) checkFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, []spec.DriftDetail, error) {
		panic(msg)
	}
}

func panicExecFn(msg string) execFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		panic(msg)
	}
}

func (o fakeOp) Action() spec.Action  { return o.action }
func (o fakeOp) DependsOn() []spec.Op { return o.deps }

func (o *fakeOp) Check(
	ctx context.Context,
	src source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	o.checkCalls++
	return o.checkFn(ctx, src, tgt)
}

func (o *fakeOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	o.execCalls++
	return o.execFn(ctx, src, tgt)
}

func (o fakeOp) OpDescription() spec.OpDescription {
	return o
}

func (o fakeOp) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{ID: o.name}
}

func (fakeOp) RequiredCapabilities() capability.Capability {
	return capability.POSIX
}

type fakeAction struct {
	ops []spec.Op
}

func (fakeAction) Kind() string     { return "fakeActionKind" }
func (fakeAction) Desc() string     { return "fakeAction" }
func (a fakeAction) Ops() []spec.Op { return a.ops }

func mkAction(ops ...*fakeOp) *fakeAction {
	act := &fakeAction{}

	for _, op := range ops {
		act.ops = append(act.ops, op)
		op.action = act
	}

	return act
}

type fakeDiagnostic struct {
	severity signal.Severity
	impact   diagnostic.Impact
	cause    error // optional underlying error
}

func (d fakeDiagnostic) Error() string {
	if d.cause != nil {
		return d.cause.Error()
	}
	return "fake diagnostic"
}

func (d fakeDiagnostic) Unwrap() error { return d.cause }

func (d fakeDiagnostic) EventTemplate() event.Template {
	return event.Template{
		ID:   "test.FakeDiagnostic",
		Text: "{{if .}}{{.}}{{else}}test diagnostic{{end}}",
		Data: d.cause,
	}
}

func (d fakeDiagnostic) Severity() signal.Severity { return d.severity }
func (d fakeDiagnostic) Impact() diagnostic.Impact { return d.impact }

// stubStepType returns a pre-built action from Plan, bypassing real config
// parsing. Useful when tests need to control exactly which ops are planned.
type stubStepType struct {
	kind   string
	action spec.Action
}

func (s *stubStepType) Kind() string   { return s.kind }
func (s *stubStepType) NewConfig() any { return &struct{}{} }
func (s *stubStepType) Plan(_ spec.StepInstance) (spec.Action, error) {
	return s.action, nil
}

// inspectableFakeOp is a fakeOp that also implements spec.Diffable.
type inspectableFakeOp struct {
	fakeOp
	desired []byte
	current []byte
	currErr error
	dest    string
}

func (o *inspectableFakeOp) DesiredContent(_ context.Context, _ source.Source) ([]byte, error) {
	return o.desired, nil
}

func (o *inspectableFakeOp) CurrentContent(_ context.Context, _ source.Source, _ target.Target) ([]byte, error) {
	return o.current, o.currErr
}

func (o *inspectableFakeOp) DestPath() string {
	return o.dest
}
