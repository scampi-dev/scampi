package copy

import (
	"bytes"
	"context"
	"io/fs"
	"path/filepath"

	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"godoit.dev/doit/util"
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
		unit  spec.UnitInstance
	}
)

func (Copy) Kind() string   { return "copy" }
func (Copy) NewConfig() any { return &CopyConfig{} }

func (c Copy) Plan(idx int, unit spec.UnitInstance) (spec.Action, error) {
	cfg, ok := unit.Config.(*CopyConfig)
	if !ok {
		return nil, util.BUG("expected %T got %T", &CopyConfig{}, unit.Config)
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

		unit: unit,
	}, nil
}

func (c *copyAction) Name() string { return c.name }
func (c *copyAction) Kind() string { return c.kind }

func (c *copyAction) Ops() []spec.Op {
	cp := &copyFileOp{
		baseOp: baseOp{
			srcSpan:  c.unit.Fields["src"].Value,
			destSpan: c.unit.Fields["dest"].Value,
		},
		src:  c.src,
		dest: c.dest,
	}
	chown := &ensureOwnerOp{
		baseOp: baseOp{
			destSpan: c.unit.Fields["dest"].Value,
		},
		path:  c.dest,
		owner: c.owner,
		group: c.group,
	}
	chmod := &ensureModeOp{
		baseOp: baseOp{
			destSpan: c.unit.Fields["dest"].Value,
		},
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
		action   spec.Action
		deps     []spec.Op
		srcSpan  spec.SourceSpan
		destSpan spec.SourceSpan
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
	// source must exist
	srcData, err := src.ReadFile(ctx, op.src)
	if err != nil {
		return spec.CheckUnsatisfied, CopySourceMissing{
			Path:   op.src,
			Err:    err,
			Source: op.srcSpan,
		}
	}

	// destination parent must exist
	if _, err := tgt.Stat(ctx, filepath.Dir(op.dest)); err != nil {
		return spec.CheckUnsatisfied, CopyDestDirMissing{
			Path:   filepath.Dir(op.dest),
			Err:    err,
			Source: op.destSpan,
		}
	}

	// dest file comparison (expected drift)
	destData, err := tgt.ReadFile(ctx, op.dest)
	if err != nil {
		return spec.CheckUnsatisfied, nil
	}

	if !bytes.Equal(srcData, destData) {
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
func (op *ensureOwnerOp) Check(ctx context.Context, _ source.Source, tgt target.Target) (spec.CheckResult, error) {
	have, err := tgt.GetOwner(ctx, op.path)
	if err != nil {
		if target.IsNotExist(err) {
			// file missing -> expected drift, copyFileOp will create it
			return spec.CheckUnsatisfied, nil
		}

		// non-transient error (perm, IO, etc.) -> abort
		return spec.CheckUnsatisfied, OwnerReadError{
			Path:   op.path,
			Err:    err,
			Source: op.destSpan,
		}
	}

	if have.User != op.owner || have.Group != op.group {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *ensureOwnerOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	have, err := tgt.GetOwner(ctx, op.path)
	if err != nil {
		if target.IsNotExist(err) {
			// file should exist - copyFileOp is a dependency and should have created it
			panic(util.BUG("ensureOwnerOp.Execute: file %q does not exist after copyFileOp", op.path))
		}

		return spec.Result{}, OwnerReadError{
			Path:   op.path,
			Err:    err,
			Source: op.destSpan,
		}
	}

	changed := have.User != op.owner || have.Group != op.group

	if err := tgt.Chown(ctx, op.path, target.Owner{User: op.owner, Group: op.group}); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: changed}, nil
}

func (op *ensureModeOp) Name() string { return "ensureModeOp" }
func (op *ensureModeOp) Check(ctx context.Context, _ source.Source, tgt target.Target) (spec.CheckResult, error) {
	info, err := tgt.Stat(ctx, op.path)
	if err != nil {
		if target.IsNotExist(err) {
			// file missing -> expected drift, copyFileOp will create it
			return spec.CheckUnsatisfied, nil
		}

		// non-transient error (perm, IO, etc.) -> abort
		return spec.CheckUnsatisfied, ModeReadError{
			Path:   op.path,
			Err:    err,
			Source: op.destSpan,
		}
	}

	if info.Mode() != op.mode {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *ensureModeOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	info, err := tgt.Stat(ctx, op.path)
	if err != nil {
		if target.IsNotExist(err) {
			// file should exist - copyFileOp is a dependency and should have created it
			panic(util.BUG("ensureModeOp.Execute: file %q does not exist after copyFileOp", op.path))
		}

		return spec.Result{}, ModeReadError{
			Path:   op.path,
			Err:    err,
			Source: op.destSpan,
		}
	}

	changed := info.Mode() != op.mode

	if err := tgt.Chmod(ctx, op.path, op.mode); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: changed}, nil
}
