// SPDX-License-Identifier: GPL-3.0-only

package dir

import (
	"context"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/step/sharedop/fileop"
	"scampi.dev/scampi/target"
)

const ensureDirID = "step.dir"

type (
	Dir       struct{}
	DirConfig struct {
		_ struct{} `summary:"Ensure a directory exists with optional permissions and ownership"`

		Desc     string   `step:"Human-readable description" optional:"true"`
		Path     string   `step:"Absolute path to ensure exists (creates parents)" example:"/opt/app/data"`
		Perm     string   `step:"File permissions" optional:"true" example:"0755|u=rwx,g=r-x,o=r-x|rwxr-xr-x"`
		Owner    string   `step:"Owner user name or UID" optional:"true" example:"root"`
		Group    string   `step:"Owner group name or GID" optional:"true" example:"root"`
		Promises []string `step:"Cross-deploy resources this step produces" optional:"true"`
		Inputs   []string `step:"Cross-deploy resources this step consumes" optional:"true"`
	}
	dirAction struct {
		desc string
		kind string
		path string
		step spec.StepInstance
	}
)

func (Dir) Kind() string   { return "dir" }
func (Dir) NewConfig() any { return &DirConfig{} }

func (c *DirConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (d Dir) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*DirConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &DirConfig{}, step.Config)
	}

	// Path absoluteness and perm format are validated at link time
	// by @std.path(absolute=true) and @std.filemode on the stub.
	// Owner/group mutual requirement is cross-field — stays here.
	if cfg.Owner != "" && cfg.Group == "" {
		return nil, PartialOwnershipError{
			Set: "owner", Missing: "group",
			Source: step.Fields["owner"].Value,
		}
	}
	if cfg.Group != "" && cfg.Owner == "" {
		return nil, PartialOwnershipError{
			Set: "group", Missing: "owner",
			Source: step.Fields["group"].Value,
		}
	}

	return &dirAction{
		desc: cfg.Desc,
		kind: d.Kind(),
		path: cfg.Path,
		step: step,
	}, nil
}

func (a *dirAction) Desc() string { return a.desc }
func (a *dirAction) Kind() string { return a.kind }

func (a *dirAction) Inputs() []spec.Resource {
	cfg := a.step.Config.(*DirConfig)
	var r []spec.Resource
	if cfg.Owner != "" {
		r = append(r, spec.UserResource(cfg.Owner))
	}
	if cfg.Group != "" {
		r = append(r, spec.GroupResource(cfg.Group))
	}
	return r
}
func (a *dirAction) Promises() []spec.Resource { return []spec.Resource{spec.PathResource(a.path)} }

func (a *dirAction) Ops() []spec.Op {
	cfg := a.step.Config.(*DirConfig)

	dir := &ensureDirOp{
		path:     a.path,
		pathSpan: a.step.Fields["path"].Value,
	}
	dir.SetAction(a)

	ops := []spec.Op{dir}

	if cfg.Perm != "" {
		mode, _ := fileop.ParsePerm(cfg.Perm, a.step.Fields["perm"].Value)
		chmod := &fileop.EnsureModeOp{
			BaseOp: sharedop.BaseOp{
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
		chown := &fileop.EnsureOwnerOp{
			BaseOp: sharedop.BaseOp{
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
	sharedop.BaseOp
	path     string
	pathSpan spec.SourceSpan
}

func (op *ensureDirOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fsTgt := target.Must[target.Filesystem](ensureDirID, tgt)

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
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	fsTgt := target.Must[target.Filesystem](ensureDirID, tgt)

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
			return spec.Result{}, sharedop.PermissionDeniedError{
				Operation: "mkdir " + op.path,
				Source:    op.pathSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedop.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: true}, nil
}

func (ensureDirOp) RequiredCapabilities() capability.Capability {
	return capability.Filesystem
}
