package copy

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"reflect"

	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

type (
	Copy       struct{}
	CopyConfig struct {
		Name  string
		Src   string
		Dest  string
		Perm  string
		Owner string
		Group string
	}
	copyAction struct {
		idx   int
		name  string
		kind  string
		src   string
		dest  string
		mode  fs.FileMode
		owner string
		group string
	}
)

func (Copy) Kind() string   { return "copy" }
func (Copy) NewConfig() any { return &CopyConfig{} }

func (c Copy) Plan(idx int, unit spec.UnitInstance) (spec.Action, error) {
	cfg, ok := unit.Config.(*CopyConfig)
	if !ok {
		return nil, fmt.Errorf("expected %T got %T", &CopyConfig{}, unit.Config)
	}

	mode, err := parsePerm(cfg.Perm, unit.Fields["perm"].Value)
	if err != nil {
		return nil, err
	}

	return &copyAction{
		idx:   idx,
		name:  cfg.Name,
		kind:  c.Kind(),
		src:   cfg.Src,
		dest:  cfg.Dest,
		mode:  mode,
		owner: cfg.Owner,
		group: cfg.Group,
	}, nil
}

func (c *copyAction) Name() string { return c.name }
func (c *copyAction) Kind() string { return c.kind }

func (c *copyAction) Ops() []spec.Op {
	cp := &copyFileOp{
		src:  c.src,
		dest: c.dest,
	}
	chown := &ensureOwnerOp{
		path:  c.dest,
		owner: c.owner,
		group: c.group,
	}
	chmod := &ensureModeOp{
		path: c.dest,
		mode: c.mode,
	}

	cp.setAction(c)
	chown.setAction(c)
	chmod.setAction(c)

	chown.addDependency(cp)
	chmod.addDependency(cp)

	return []spec.Op{
		cp,
		chown,
		chmod,
	}
}

type (
	baseOp struct {
		action spec.Action
		deps   []spec.Op
	}
	copyFileOp struct {
		baseOp
		src  string
		dest string
	}
	ensureOwnerOp struct {
		baseOp
		path  string
		owner string
		group string
	}
	ensureModeOp struct {
		baseOp
		path string
		mode fs.FileMode
	}
)

func (op *baseOp) Action() spec.Action          { return op.action }
func (op *baseOp) DependsOn() []spec.Op         { return op.deps }
func (op *baseOp) addDependency(dep spec.Op)    { op.deps = append(op.deps, dep) }
func (op *baseOp) setAction(action spec.Action) { op.action = action }

func (op *copyFileOp) Name() string { return "copyFileOp" }
func (op *copyFileOp) Check(ctx context.Context, src source.Source, tgt target.Target) (spec.CheckResult, error) {
	srcData, err := src.ReadFile(ctx, op.src)
	if err != nil {
		// fail if src file does not exist or is unreadable or whatever
		// probably better using STAT in the future, but oh well
		return spec.CheckUnsatisfied, err
	}

	destData, err := tgt.ReadFile(ctx, op.dest)
	if err != nil || !bytes.Equal(srcData, destData) {
		// we do not fail the playbook if dest doesn't exist, is unreadable, etc.
		// we're just indicating that copyFileOp needs to run
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *copyFileOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	srcData, err := src.ReadFile(ctx, op.src)
	if err != nil {
		return spec.Result{}, err
	}

	destData, err := tgt.ReadFile(ctx, op.dest)
	if err == nil && bytes.Equal(srcData, destData) {
		return spec.Result{Changed: false}, nil
	}

	if err := tgt.WriteFile(ctx, op.dest, srcData, 0o644); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}

func (op *ensureOwnerOp) Name() string { return "ensureOwnerOp" }
func (op *ensureOwnerOp) Check(ctx context.Context, src source.Source, tgt target.Target) (spec.CheckResult, error) {
	haveOwner, err := tgt.GetOwner(ctx, op.path)
	if err != nil {
		return spec.CheckUnknown, err
	}

	wantOwner := target.Owner{User: op.owner, Group: op.group}
	// TODO: this is probably a target concern
	if !reflect.DeepEqual(haveOwner, wantOwner) {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *ensureOwnerOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	if err := tgt.Chown(ctx, op.path, target.Owner{User: op.owner, Group: op.group}); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}

func (op *ensureModeOp) Name() string { return "ensureModeOp" }
func (op *ensureModeOp) Check(ctx context.Context, src source.Source, tgt target.Target) (spec.CheckResult, error) {
	info, err := tgt.Stat(ctx, op.path)
	if err != nil {
		return spec.CheckUnsatisfied, nil
	}

	want := info.Mode()
	if want != op.mode {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *ensureModeOp) Execute(ctx context.Context, src source.Source, tgt target.Target) (spec.Result, error) {
	if err := tgt.Chmod(ctx, op.path, op.mode); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}
