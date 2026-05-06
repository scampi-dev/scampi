// SPDX-License-Identifier: GPL-3.0-only

package symlink

import (
	"context"
	"io/fs"
	"path/filepath"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const ensureSymlinkID = "step.symlink"

type (
	Symlink       struct{}
	SymlinkConfig struct {
		_ struct{} `summary:"Create and manage symbolic links"`

		Desc     string   `step:"Human-readable description" optional:"true"`
		Target   string   `step:"Path the symlink points to (like ln -s TARGET)" example:"/opt/app/config.yaml"`
		Link     string   `step:"Path where symlink is created (like ln -s ... LINK)" example:"/etc/app/config.yaml"`
		Promises []string `step:"Cross-deploy resources this step produces" optional:"true"`
		Inputs   []string `step:"Cross-deploy resources this step consumes" optional:"true"`
	}
	symlinkAction struct {
		desc   string
		kind   string
		target string
		link   string
		step   spec.StepInstance
	}
)

func (Symlink) Kind() string   { return "symlink" }
func (Symlink) NewConfig() any { return &SymlinkConfig{} }

func (c *SymlinkConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (s Symlink) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*SymlinkConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &SymlinkConfig{}, step.Config)
	}

	// Link absoluteness is validated at link time by
	// @std.path(absolute=true) on symlink.link in the stub.
	return &symlinkAction{
		desc:   cfg.Desc,
		kind:   s.Kind(),
		target: cfg.Target,
		link:   cfg.Link,
		step:   step,
	}, nil
}

func (a *symlinkAction) Desc() string { return a.desc }
func (a *symlinkAction) Kind() string { return a.kind }
func (a *symlinkAction) Inputs() []spec.Resource {
	return []spec.Resource{spec.PathResource(a.target)}
}
func (a *symlinkAction) Promises() []spec.Resource {
	return []spec.Resource{spec.PathResource(a.link)}
}
func (a *symlinkAction) Ops() []spec.Op {
	op := &ensureSymlinkOp{
		BaseOp: sharedop.BaseOp{
			SrcSpan:  a.step.Fields["target"].Value,
			DestSpan: a.step.Fields["link"].Value,
		},
		target: a.target,
		link:   a.link,
	}

	op.SetAction(a)

	return []spec.Op{op}
}

type ensureSymlinkOp struct {
	sharedop.BaseOp
	target string
	link   string
}

// resolveTarget computes the symlink target path.
// If target is absolute, it's used as-is.
// If target is relative (to cwd), it's converted to be relative to the link's directory.
func resolveTarget(target, link string) (string, error) {
	if filepath.IsAbs(target) {
		return target, nil
	}

	// Convert relative target to absolute (based on cwd)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}

	// Get absolute link directory
	absLink, err := filepath.Abs(link)
	if err != nil {
		return "", err
	}
	linkDir := filepath.Dir(absLink)

	return filepath.Rel(linkDir, absTarget)
}

func (op *ensureSymlinkOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	t := target.Must[interface {
		target.Filesystem
		target.Symlink
	}](ensureSymlinkID, tgt)

	if _, err := t.Stat(ctx, filepath.Dir(op.link)); err != nil {
		return spec.CheckUnsatisfied, nil, LinkDirMissingError{
			Path:   filepath.Dir(op.link),
			Err:    err,
			Source: op.DestSpan,
		}
	}

	relTarget, err := resolveTarget(op.target, op.link)
	if err != nil {
		return spec.CheckUnsatisfied, nil, LinkReadError{
			Path:   op.link,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	info, err := t.Lstat(ctx, op.link)
	if err != nil {
		if target.IsNotExist(err) {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "target",
				Desired: relTarget,
			}}, nil
		}

		return spec.CheckUnsatisfied, nil, LinkReadError{
			Path:   op.link,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	// Non-symlink at link path → drift. Remove + symlink in Execute.
	// Refusing here forced users into `posix.run { ln -sf ... }` for
	// the common case where a package dropped a stock config and we
	// want to replace it with a symlink to the real one (#279).
	if info.Mode()&fs.ModeSymlink == 0 {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "target",
			Current: describeNonSymlink(info),
			Desired: relTarget,
		}}, nil
	}

	current, err := t.Readlink(ctx, op.link)
	if err != nil {
		return spec.CheckUnsatisfied, nil, LinkReadError{
			Path:   op.link,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	if current != relTarget {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "target",
			Current: current,
			Desired: relTarget,
		}}, nil
	}

	return spec.CheckSatisfied, nil, nil
}

// describeNonSymlink returns a human-readable label for the entry at
// the link path when it isn't a symlink — used as the "current" value
// in drift details.
func describeNonSymlink(info fs.FileInfo) string {
	switch {
	case info.IsDir():
		return "directory"
	case info.Mode().IsRegular():
		return "regular file"
	default:
		return info.Mode().String()
	}
}

func (op *ensureSymlinkOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	t := target.Must[interface {
		target.Filesystem
		target.Symlink
	}](ensureSymlinkID, tgt)

	relTarget, err := resolveTarget(op.target, op.link)
	if err != nil {
		return spec.Result{}, err
	}

	info, err := t.Lstat(ctx, op.link)
	if err == nil {
		// Existing entry. If it's already the symlink we want, done.
		// Otherwise remove and recreate (regular file, wrong-target
		// symlink, empty directory). Remove() refuses to recursively
		// delete a non-empty directory — the filesystem error
		// surfaces with the dir intact.
		if info.Mode()&fs.ModeSymlink != 0 {
			current, _ := t.Readlink(ctx, op.link)
			if current == relTarget {
				return spec.Result{Changed: false}, nil
			}
		}
		if err := t.Remove(ctx, op.link); err != nil {
			if target.IsPermission(err) {
				return spec.Result{}, sharedop.PermissionDeniedError{
					Operation: "remove " + op.link,
					Source:    op.DestSpan,
					Err:       err,
				}
			}
			return spec.Result{}, sharedop.DiagnoseTargetError(err)
		}
	}

	// Create symlink
	if err := t.Symlink(ctx, relTarget, op.link); err != nil {
		if target.IsPermission(err) {
			return spec.Result{}, sharedop.PermissionDeniedError{
				Operation: "symlink " + op.link,
				Source:    op.DestSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedop.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: true}, nil
}

func (ensureSymlinkOp) RequiredCapabilities() capability.Capability {
	return capability.Filesystem | capability.Symlink
}
