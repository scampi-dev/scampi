package fileops

import (
	"context"
	"fmt"
	"io/fs"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/target"
)

const ensureModeID = "builtin.ensure-mode"

type EnsureModeOp struct {
	sharedops.BaseOp
	Path string
	Mode fs.FileMode
}

func (op *EnsureModeOp) Check(
	ctx context.Context, _ source.Source, tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fsTgt := target.Must[target.Filesystem](ensureModeID, tgt)

	info, err := fsTgt.Stat(ctx, op.Path)
	if err != nil {
		if target.IsNotExist(err) {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "perm",
				Desired: op.Mode.String(),
			}}, nil
		}

		return spec.CheckUnsatisfied, nil, modeReadError{
			Path:   op.Path,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	if info.Mode() != op.Mode {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "perm",
			Current: info.Mode().String(),
			Desired: op.Mode.String(),
		}}, nil
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *EnsureModeOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	fsTgt := target.Must[target.Filesystem](ensureModeID, tgt)
	fmTgt := target.Must[target.FileMode](ensureModeID, tgt)

	info, err := fsTgt.Stat(ctx, op.Path)
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

	if err := fmTgt.Chmod(ctx, op.Path, op.Mode); err != nil {
		// Can't catch during Check: file may not exist yet, and probing
		// write-permission would mutate state in a read-only phase.
		if target.IsPermission(err) {
			return spec.Result{}, sharedops.PermissionDeniedError{
				Operation: fmt.Sprintf("chmod %s %s", op.Mode, op.Path),
				Source:    op.DestSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: changed}, nil
}

func (EnsureModeOp) RequiredCapabilities() capability.Capability {
	return capability.FileMode
}

type ensureModeDesc struct {
	Mode string
	Path string
}

func (d ensureModeDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureModeID,
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
		ID:     "builtin.ModeRead",
		Text:   `cannot read mode of "{{.Path}}"`,
		Hint:   "check file permissions and ensure the path is accessible",
		Data:   e,
		Source: &e.Source,
	}
}

func (modeReadError) Severity() signal.Severity { return signal.Error }
func (modeReadError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
