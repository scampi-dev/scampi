package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

type ExpectedDiagnostics struct {
	Abort       bool                 `json:"abort"`
	Diagnostics []ExpectedDiagnostic `json:"diagnostics"`
}

type ExpectedDiagnostic struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Scope    string `json:"scope"`
	Severity string `json:"severity"`

	Source *ExpectedSource `json:"source,omitempty"`
	Unit   *ExpectedUnit   `json:"unit,omitempty"`
}

type ExpectedSource struct {
	Line int `json:"line"`
}

type ExpectedUnit struct {
	Index int    `json:"index"`
	Kind  string `json:"kind"`
}

type (
	events             []event.Event
	recordingDisplayer struct {
		mu     sync.Mutex
		events events
	}
	noopEmitter struct{}
)

func (r *recordingDisplayer) Emit(e event.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recordingDisplayer) Close() {}

func (r *recordingDisplayer) String() string {
	return r.events.String()
}

func (r *recordingDisplayer) dump(w io.Writer) {
	_, _ = fmt.Fprintln(w, r)
}

func (e events) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- DIAGNOSTICS -----\n" +
		string(j)
}

func (noopEmitter) Emit(event.Event) {}

type (
	checkFn func(context.Context, source.Source, target.Target) (spec.CheckResult, error)
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
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
		return res, nil
	}
}

func okExecFn(changed bool) execFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		return spec.Result{Changed: changed}, nil
	}
}

func diagCheckFn(severity signal.Severity, impact diagnostic.Impact) checkFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
		return spec.CheckUnknown, &fakeDiagnostic{
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
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
		return spec.CheckUnknown, err
	}
}

//lint:ignore U1000
func errExecFn(err error) execFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		return spec.Result{}, err
	}
}

func panicCheckFn(msg string) checkFn {
	return func(context.Context, source.Source, target.Target) (spec.CheckResult, error) {
		panic(msg)
	}
}

func panicExecFn(msg string) execFn {
	return func(context.Context, source.Source, target.Target) (spec.Result, error) {
		panic(msg)
	}
}

func (o fakeOp) Name() string         { return o.name }
func (o fakeOp) Action() spec.Action  { return o.action }
func (o fakeOp) DependsOn() []spec.Op { return o.deps }
func (o *fakeOp) Check(ctx context.Context, src source.Source, tgt target.Target) (spec.CheckResult, error) {
	o.checkCalls++
	return o.checkFn(ctx, src, tgt)
}

func (o *fakeOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	o.execCalls++
	return o.execFn(ctx, src, tgt)
}

type fakeAction struct {
	ops []spec.Op
}

func (fakeAction) Kind() string     { return "fakeActionKind" }
func (fakeAction) Name() string     { return "fakeAction" }
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
}

func (fakeDiagnostic) Error() string { return "fake diagnostic" }

func (fakeDiagnostic) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, fakeDiagnostic{}),
	}
}

func (fakeDiagnostic) EventTemplate() event.Template {
	return event.Template{
		ID:   "test.FakeDiagnostic",
		Text: "test diagnostic",
	}
}

func (d fakeDiagnostic) Severity() signal.Severity {
	return d.severity
}

func (d fakeDiagnostic) Impact() diagnostic.Impact {
	return d.impact
}

func absPath(p string) string {
	r, err := filepath.Abs(p)
	if err != nil {
		panic(err)
	}

	return r
}

func readDirOrDie(name string) []os.DirEntry {
	res, err := os.ReadDir(name)
	if err != nil {
		panic(err)
	}

	return res
}

func readOrDie(name string) []byte {
	data, err := os.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return data
}

func writeOrDie(name string, data []byte, perm os.FileMode) {
	if err := os.WriteFile(name, data, perm); err != nil {
		panic(err)
	}
}
