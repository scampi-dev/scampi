package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"godoit.dev/doit/capability"
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
	Step   *ExpectedStep   `json:"step,omitempty"`
}

type ExpectedSource struct {
	Line int `json:"line"`
}

type ExpectedStep struct {
	Index int    `json:"index"`
	Kind  string `json:"kind"`
}

type (
	engineEvents       []event.EngineEvent
	planEvents         []event.PlanEvent
	actionEvents       []event.ActionEvent
	opEvents           []event.OpEvent
	indexAllEvents     []event.IndexAllEvent
	engineDiagnostics  []event.EngineDiagnostic
	planDiagnostics    []event.PlanDiagnostic
	actionDiagnostics  []event.ActionDiagnostic
	opDiagnostics      []event.OpDiagnostic
	indexStepEvents    []event.IndexStepEvent
	recordingDisplayer struct {
		mu                sync.Mutex
		engineEvents      engineEvents
		planEvents        planEvents
		actionEvents      actionEvents
		opEvents          opEvents
		engineDiagnostics engineDiagnostics
		planDiagnostics   planDiagnostics
		actionDiagnostics actionDiagnostics
		opDiagnostics     opDiagnostics
		indexAllEvents    indexAllEvents
		indexStepEvents   indexStepEvents
	}
	noopEmitter struct{}
)

func (r *recordingDisplayer) EmitEngineLifecycle(e event.EngineEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engineEvents = append(r.engineEvents, e)
}

func (r *recordingDisplayer) EmitPlanLifecycle(e event.PlanEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.planEvents = append(r.planEvents, e)
}

func (r *recordingDisplayer) EmitActionLifecycle(e event.ActionEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actionEvents = append(r.actionEvents, e)
}

func (r *recordingDisplayer) EmitOpLifecycle(e event.OpEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opEvents = append(r.opEvents, e)
}

func (r *recordingDisplayer) EmitEngineDiagnostic(e event.EngineDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engineDiagnostics = append(r.engineDiagnostics, e)
}

func (r *recordingDisplayer) EmitPlanDiagnostic(e event.PlanDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.planDiagnostics = append(r.planDiagnostics, e)
}

func (r *recordingDisplayer) EmitActionDiagnostic(e event.ActionDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actionDiagnostics = append(r.actionDiagnostics, e)
}

func (r *recordingDisplayer) EmitOpDiagnostic(e event.OpDiagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opDiagnostics = append(r.opDiagnostics, e)
}

func (r *recordingDisplayer) EmitIndexAll(e event.IndexAllEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indexAllEvents = append(r.indexAllEvents, e)
}

func (r *recordingDisplayer) EmitIndexStep(e event.IndexStepEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indexStepEvents = append(r.indexStepEvents, e)
}

func (r *recordingDisplayer) Close() {}

func (r *recordingDisplayer) String() string {
	return r.engineEvents.String() + "\n" +
		r.planEvents.String() + "\n" +
		r.actionEvents.String() + "\n" +
		r.opEvents.String() + "\n" +
		r.indexAllEvents.String() + "\n" +
		r.indexStepEvents.String() + "\n" +
		r.engineDiagnostics.String() + "\n" +
		r.planDiagnostics.String() + "\n" +
		r.actionDiagnostics.String() + "\n" +
		r.opDiagnostics.String()
}

func (r *recordingDisplayer) dump(w io.Writer) {
	_, _ = fmt.Fprintln(w, r)
}

func (r *recordingDisplayer) countChangedOps() int {
	count := 0
	for _, ev := range r.opEvents {
		if ev.ExecuteDetail != nil && ev.ExecuteDetail.Changed {
			count++
		}
	}
	return count
}

func (r *recordingDisplayer) collectDiagnosticIDs() []string {
	var ids []string
	for _, d := range r.engineDiagnostics {
		ids = append(ids, d.Detail.Template.ID)
	}
	for _, d := range r.planDiagnostics {
		ids = append(ids, d.Detail.Template.ID)
	}
	for _, d := range r.actionDiagnostics {
		ids = append(ids, d.Detail.Template.ID)
	}
	for _, d := range r.opDiagnostics {
		ids = append(ids, d.Detail.Template.ID)
	}
	return ids
}

func (e engineEvents) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- ENGINE DIAGNOSTICS -----\n" +
		string(j)
}

func (e planEvents) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- PLAN DIAGNOSTICS -----\n" +
		string(j)
}

func (e actionEvents) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- ACTION DIAGNOSTICS -----\n" +
		string(j)
}

func (e opEvents) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- OP EVENTS -----\n" +
		string(j)
}

func (e indexAllEvents) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- INDEX_ALL EVENTS -----\n" +
		string(j)
}

func (e indexStepEvents) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- INDEX_STEP EVENTS -----\n" +
		string(j)
}

func (e engineDiagnostics) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- ENGINE DIAGNOSTICS -----\n" +
		string(j)
}

func (e planDiagnostics) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- PLAN DIAGNOSTICS (DIAGS) -----\n" +
		string(j)
}

func (e actionDiagnostics) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- ACTION DIAGNOSTICS (DIAGS) -----\n" +
		string(j)
}

func (e opDiagnostics) String() string {
	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		panic(err)
	}
	return "----- OP DIAGNOSTICS (DIAGS) -----\n" +
		string(j)
}

func (noopEmitter) EmitEngineLifecycle(event.EngineEvent)       {}
func (noopEmitter) EmitPlanLifecycle(event.PlanEvent)           {}
func (noopEmitter) EmitActionLifecycle(event.ActionEvent)       {}
func (noopEmitter) EmitOpLifecycle(event.OpEvent)               {}
func (noopEmitter) EmitEngineDiagnostic(event.EngineDiagnostic) {}
func (noopEmitter) EmitPlanDiagnostic(event.PlanDiagnostic)     {}
func (noopEmitter) EmitActionDiagnostic(event.ActionDiagnostic) {}
func (noopEmitter) EmitOpDiagnostic(event.OpDiagnostic)         {}
func (noopEmitter) EmitIndexAll(event.IndexAllEvent)            {}
func (noopEmitter) EmitIndexStep(event.IndexStepEvent)          {}

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
	text := "test diagnostic"
	if d.cause != nil {
		text = d.cause.Error()
	}
	return event.Template{
		ID:   "test.FakeDiagnostic",
		Text: text,
	}
}

func (d fakeDiagnostic) Severity() signal.Severity { return d.severity }
func (d fakeDiagnostic) Impact() diagnostic.Impact { return d.impact }

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

func readDirSafe(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func readFileSafe(name string) ([]byte, error) {
	return os.ReadFile(name)
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

// faultySource wraps a source.Source and injects errors on configured paths.
type faultySource struct {
	source.Source

	mu     sync.RWMutex
	faults map[string]error
}

func newFaultySource(inner source.Source) *faultySource {
	return &faultySource{
		Source: inner,
		faults: make(map[string]error),
	}
}

func (f *faultySource) injectFault(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults[path] = &fakeDiagnostic{
		severity: signal.Error,
		impact:   diagnostic.ImpactAbort,
		cause:    err,
	}
}

//lint:ignore U1000 kept for symmetry with faultyTarget
func (f *faultySource) clearFaults() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults = make(map[string]error)
}

func (f *faultySource) getFault(path string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.faults[path]
}

func (f *faultySource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := f.getFault(path); err != nil {
		return nil, err
	}
	return f.Source.ReadFile(ctx, path)
}

func (f *faultySource) Stat(ctx context.Context, path string) (source.FileMeta, error) {
	if err := f.getFault(path); err != nil {
		return source.FileMeta{}, err
	}
	return f.Source.Stat(ctx, path)
}

// faultyTarget wraps a target.Target and injects errors on configured method/path pairs.
type faultyTarget struct {
	target.Target

	mu     sync.RWMutex
	faults map[faultKey]error
}

type faultKey struct {
	method string
	path   string
}

func newFaultyTarget(inner target.Target) *faultyTarget {
	return &faultyTarget{
		Target: inner,
		faults: make(map[faultKey]error),
	}
}

func (f *faultyTarget) injectFault(method, path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults[faultKey{method, path}] = &fakeDiagnostic{
		severity: signal.Error,
		impact:   diagnostic.ImpactAbort,
		cause:    err,
	}
}

func (f *faultyTarget) clearFaults() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults = make(map[faultKey]error)
}

func (f *faultyTarget) getFault(method, path string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.faults[faultKey{method, path}]
}

func (f *faultyTarget) Stat(ctx context.Context, path string) (fs.FileInfo, error) {
	if err := f.getFault("Stat", path); err != nil {
		return nil, err
	}
	return target.Must[target.Filesystem]("faultyTarget", f.Target).Stat(ctx, path)
}

func (f *faultyTarget) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := f.getFault("ReadFile", path); err != nil {
		return nil, err
	}
	return target.Must[target.Filesystem]("faultyTarget", f.Target).ReadFile(ctx, path)
}

func (f *faultyTarget) WriteFile(ctx context.Context, path string, data []byte) error {
	if err := f.getFault("WriteFile", path); err != nil {
		return err
	}
	return target.Must[target.Filesystem]("faultyTarget", f.Target).WriteFile(ctx, path, data)
}

func (f *faultyTarget) Remove(ctx context.Context, path string) error {
	if err := f.getFault("Remove", path); err != nil {
		return err
	}
	return target.Must[target.Filesystem]("faultyTarget", f.Target).Remove(ctx, path)
}

func (f *faultyTarget) Chmod(ctx context.Context, path string, mode fs.FileMode) error {
	if err := f.getFault("Chmod", path); err != nil {
		return err
	}
	return target.Must[target.FileMode]("faultyTarget", f.Target).Chmod(ctx, path, mode)
}

func (f *faultyTarget) Chown(ctx context.Context, path string, owner target.Owner) error {
	if err := f.getFault("Chown", path); err != nil {
		return err
	}
	return target.Must[target.Ownership]("faultyTarget", f.Target).Chown(ctx, path, owner)
}

func (f *faultyTarget) GetOwner(ctx context.Context, path string) (target.Owner, error) {
	if err := f.getFault("GetOwner", path); err != nil {
		return target.Owner{}, err
	}
	return target.Must[target.Ownership]("faultyTarget", f.Target).GetOwner(ctx, path)
}

func (f *faultyTarget) HasUser(ctx context.Context, user string) bool {
	return target.Must[target.Ownership]("faultyTarget", f.Target).HasUser(ctx, user)
}

func (f *faultyTarget) HasGroup(ctx context.Context, group string) bool {
	return target.Must[target.Ownership]("faultyTarget", f.Target).HasGroup(ctx, group)
}

type minimalTarget struct {
	*target.MemTarget
}

func newMinimalTarget() *minimalTarget {
	return &minimalTarget{MemTarget: target.NewMemTarget()}
}

func (m *minimalTarget) Capabilities() capability.Capability {
	return capability.Filesystem
}

func (m *minimalTarget) HasUser(_ context.Context, _ string) bool {
	panic("MinimalTarget.HasUser called - capability check failed")
}

func (m *minimalTarget) HasGroup(_ context.Context, _ string) bool {
	panic("MinimalTarget.HasUser called - capability check failed")
}

func (m *minimalTarget) GetOwner(_ context.Context, _ string) (target.Owner, error) {
	panic("MinimalTarget.HasUser called - capability check failed")
}

type allCapNoImplTarget struct{}

func (allCapNoImplTarget) Capabilities() capability.Capability {
	return capability.All
}

type mockTargetType struct {
	tgt target.Target
}

func (mockTargetType) Kind() string                          { return "mem" }
func (mockTargetType) NewConfig() any                        { return nil }
func (t mockTargetType) Create(_ any) (target.Target, error) { return t.tgt, nil }

func mockTargetInstance(tgt target.Target) spec.TargetInstance {
	return spec.TargetInstance{
		Type: mockTargetType{
			tgt: tgt,
		},
	}
}
