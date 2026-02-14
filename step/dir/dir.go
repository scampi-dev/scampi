// SPDX-License-Identifier: GPL-3.0-only

package dir

import (
	"context"
	"path/filepath"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/step/sharedops/fileops"
	"godoit.dev/doit/target"
)

const id = "builtin.dir"

type (
	Dir       struct{}
	DirConfig struct {
		_ struct{} `summary:"Ensure a directory exists with optional permissions and ownership"`

		Desc  string `step:"Human-readable description" optional:"true"`
		Path  string `step:"Absolute path to ensure exists (creates parents)" example:"/opt/app/data"`
		Perm  string `step:"Permissions in octal notation (e.g. 0755)" optional:"true" example:"0755"`
		Owner string `step:"Owner user name or UID" optional:"true" example:"root"`
		Group string `step:"Owner group name or GID" optional:"true" example:"root"`
	}
	dirAction struct {
		idx  int
		desc string
		kind string
		path string
		step spec.StepInstance
	}
)

func (Dir) Kind() string   { return "dir" }
func (Dir) NewConfig() any { return &DirConfig{} }

func (d Dir) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*DirConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &DirConfig{}, step.Config)
	}

	if !filepath.IsAbs(cfg.Path) {
		return nil, sharedops.RelativePathError{
			Field:  "path",
			Path:   cfg.Path,
			Source: step.Fields["path"].Value,
		}
	}

	if cfg.Perm != "" {
		if _, err := fileops.ParsePerm(cfg.Perm, step.Fields["perm"].Value); err != nil {
			return nil, err
		}
	}

	return &dirAction{
		idx:  idx,
		desc: cfg.Desc,
		kind: d.Kind(),
		path: cfg.Path,
		step: step,
	}, nil
}

func (a *dirAction) Desc() string          { return a.desc }
func (a *dirAction) Kind() string          { return a.kind }
func (a *dirAction) InputPaths() []string  { return nil }
func (a *dirAction) OutputPaths() []string { return []string{a.path} }

func (a *dirAction) Ops() []spec.Op {
	cfg := a.step.Config.(*DirConfig)

	dir := &ensureDirOp{
		path:     a.path,
		pathSpan: a.step.Fields["path"].Value,
	}
	dir.SetAction(a)

	ops := []spec.Op{dir}

	if cfg.Perm != "" {
		mode, _ := fileops.ParsePerm(cfg.Perm, a.step.Fields["perm"].Value)
		chmod := &fileops.EnsureModeOp{
			BaseOp: sharedops.BaseOp{
				DestSpan: a.step.Fields["path"].Value,
			},
			Path: a.path,
			Mode: mode,
		}
		chmod.SetAction(a)
		chmod.AddDependency(dir)
		ops = append(ops, chmod)
	}

	if cfg.Owner != "" && cfg.Group != "" {
		chown := &fileops.EnsureOwnerOp{
			BaseOp: sharedops.BaseOp{
				DestSpan: a.step.Fields["path"].Value,
			},
			Path:      a.path,
			Owner:     cfg.Owner,
			Group:     cfg.Group,
			OwnerSpan: a.step.Fields["owner"].Value,
			GroupSpan: a.step.Fields["group"].Value,
		}
		chown.SetAction(a)
		chown.AddDependency(dir)
		ops = append(ops, chown)
	}

	return ops
}

// ensureDirOp
// -----------------------------------------------------------------------------

type ensureDirOp struct {
	sharedops.BaseOp
	path     string
	pathSpan spec.SourceSpan
}

func (op *ensureDirOp) Check(
	ctx context.Context, _ source.Source, tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fsTgt := target.Must[target.Filesystem](id, tgt)

	info, err := fsTgt.Stat(ctx, op.path)
	if err != nil {
		if target.IsNotExist(err) {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "state",
				Desired: "directory",
			}}, nil
		}
		return spec.CheckUnsatisfied, nil, err
	}

	if !info.IsDir() {
		return spec.CheckUnsatisfied, nil, NotADirectoryError{
			Path:   op.path,
			Source: op.pathSpan,
		}
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *ensureDirOp) Execute(
	ctx context.Context, _ source.Source, tgt target.Target,
) (spec.Result, error) {
	fsTgt := target.Must[target.Filesystem](id, tgt)

	info, err := fsTgt.Stat(ctx, op.path)
	if err == nil {
		if info.IsDir() {
			return spec.Result{Changed: false}, nil
		}
		return spec.Result{}, NotADirectoryError{
			Path:   op.path,
			Source: op.pathSpan,
		}
	}

	if !target.IsNotExist(err) {
		return spec.Result{}, err
	}

	if err := fsTgt.Mkdir(ctx, op.path, 0o755); err != nil {
		if target.IsPermission(err) {
			return spec.Result{}, sharedops.PermissionDeniedError{
				Operation: "mkdir " + op.path,
				Source:    op.pathSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: true}, nil
}

func (ensureDirOp) RequiredCapabilities() capability.Capability {
	return capability.Filesystem
}
