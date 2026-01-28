package fileops

import (
	"context"
	"fmt"
	"io/fs"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/target"
)

type EnsureModeOp struct {
	sharedops.BaseOp
	Path string
	Mode fs.FileMode
}

func (op *EnsureModeOp) Check(ctx context.Context, _ source.Source, tgt target.Target) (spec.CheckResult, error) {
	info, err := tgt.Stat(ctx, op.Path)
	if err != nil {
		if target.IsNotExist(err) {
			// file missing -> expected drift, copyFileOp will create it
			return spec.CheckUnsatisfied, nil
		}

		// non-transient error (perm, IO, etc.) -> abort
		return spec.CheckUnsatisfied, modeReadError{
			Path:   op.Path,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	if info.Mode() != op.Mode {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *EnsureModeOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	info, err := tgt.Stat(ctx, op.Path)
	if err != nil {
		if target.IsNotExist(err) {
			// file should exist - copyFileOp is a dependency and should have created it
			panic(errs.BUG("ensureModeOp.Execute: file %q does not exist after copyFileOp", op.Path))
		}

		return spec.Result{}, modeReadError{
			Path:   op.Path,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	changed := info.Mode() != op.Mode

	if err := tgt.Chmod(ctx, op.Path, op.Mode); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: changed}, nil
}

type ensureModeDesc struct {
	Mode string
	Path string
}

func (d ensureModeDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   "builtin.ensure-mode",
		Text: `ensure mode {{.Mode}} on "{{.Path}}"`,
		Data: d,
	}
}

func (op *EnsureModeOp) OpDescription() spec.OpDescription {
	return ensureModeDesc{
		Mode: op.Mode.String(),
		Path: op.Path,
	}
}

type modeReadError struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e modeReadError) Error() string {
	return fmt.Sprintf("cannot read mode of %q: %v", e.Path, e.Err)
}

func (e modeReadError) Unwrap() error {
	return e.Err
}

func (e modeReadError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.copy.ModeReadError",
		Text:   `cannot read mode of "{{.Path}}"`,
		Hint:   "check file permissions and ensure the path is accessible",
		Data:   e,
		Source: &e.Source,
	}
}

func (modeReadError) Severity() signal.Severity { return signal.Error }
func (modeReadError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
