package symlink

import (
	"context"
	"io/fs"
	"path/filepath"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
)

type (
	Symlink       struct{}
	SymlinkConfig struct {
		Desc   string
		Target string
		Link   string
	}
	symlinkAction struct {
		idx    int
		desc   string
		kind   string
		target string
		link   string
		step   spec.StepInstance
	}
)

func (Symlink) Kind() string   { return "symlink" }
func (Symlink) NewConfig() any { return &SymlinkConfig{} }

func (s Symlink) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*SymlinkConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &SymlinkConfig{}, step.Config)
	}

	return &symlinkAction{
		idx:    idx,
		desc:   cfg.Desc,
		kind:   s.Kind(),
		target: cfg.Target,
		link:   cfg.Link,
		step:   step,
	}, nil
}

func (a *symlinkAction) Desc() string            { return a.desc }
func (a *symlinkAction) Kind() string            { return a.kind }
func (op *ensureSymlinkOp) Action() spec.Action  { return op.action }
func (op *ensureSymlinkOp) DependsOn() []spec.Op { return op.deps }

func (a *symlinkAction) Ops() []spec.Op {
	op := &ensureSymlinkOp{
		targetSpan: a.step.Fields["target"].Value,
		linkSpan:   a.step.Fields["link"].Value,
		target:     a.target,
		link:       a.link,
	}

	op.action = a

	return []spec.Op{op}
}

type (
	ensureSymlinkOp struct {
		target     string
		link       string
		action     spec.Action
		deps       []spec.Op
		targetSpan spec.SourceSpan
		linkSpan   spec.SourceSpan
	}
)

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

func (op *ensureSymlinkOp) Check(ctx context.Context, _ source.Source, tgt target.Target) (spec.CheckResult, error) {
	// Link parent dir must exist
	if _, err := tgt.Stat(ctx, filepath.Dir(op.link)); err != nil {
		return spec.CheckUnsatisfied, LinkDirMissing{
			Path:   filepath.Dir(op.link),
			Err:    err,
			Source: op.linkSpan,
		}
	}

	// Compute relative target path
	relTarget, err := resolveTarget(op.target, op.link)
	if err != nil {
		return spec.CheckUnsatisfied, LinkReadError{
			Path:   op.link,
			Err:    err,
			Source: op.linkSpan,
		}
	}

	// Check what exists at link path
	info, err := tgt.Lstat(ctx, op.link)
	if err != nil {
		if target.IsNotExist(err) {
			return spec.CheckUnsatisfied, nil // expected drift
		}

		return spec.CheckUnsatisfied, LinkReadError{
			Path:   op.link,
			Err:    err,
			Source: op.linkSpan,
		}
	}

	// Must be a symlink (not regular file/dir)
	if info.Mode()&fs.ModeSymlink == 0 {
		return spec.CheckUnsatisfied, NotASymlink{
			Path:   op.link,
			Source: op.linkSpan,
		}
	}

	// Check symlink target
	current, err := tgt.Readlink(ctx, op.link)
	if err != nil {
		return spec.CheckUnsatisfied, LinkReadError{
			Path:   op.link,
			Err:    err,
			Source: op.linkSpan,
		}
	}

	if current != relTarget {
		return spec.CheckUnsatisfied, nil // expected drift
	}

	return spec.CheckSatisfied, nil
}

func (op *ensureSymlinkOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	// Compute relative target path
	relTarget, err := resolveTarget(op.target, op.link)
	if err != nil {
		return spec.Result{}, err
	}

	// Check current state
	info, err := tgt.Lstat(ctx, op.link)
	if err == nil {
		// Something exists
		if info.Mode()&fs.ModeSymlink != 0 {
			// It's a symlink - check if correct
			current, _ := tgt.Readlink(ctx, op.link)
			if current == relTarget {
				return spec.Result{Changed: false}, nil
			}
		}

		// Remove existing (symlink with wrong target, or other file type)
		if err := tgt.Remove(ctx, op.link); err != nil {
			return spec.Result{}, err
		}
	}

	// Create symlink
	if err := tgt.Symlink(ctx, relTarget, op.link); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}

func (ensureSymlinkOp) RequiredCapabilities() capability.Capability {
	return capability.Symlink
}
