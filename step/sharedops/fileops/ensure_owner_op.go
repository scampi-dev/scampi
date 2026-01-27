package fileops

import (
	"context"
	"fmt"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/target"
	"godoit.dev/doit/util"
)

type EnsureOwnerOp struct {
	sharedops.BaseOp
	Path  string
	Owner string
	Group string
}

func (op *EnsureOwnerOp) Check(ctx context.Context, _ source.Source, tgt target.Target) (spec.CheckResult, error) {
	have, err := tgt.GetOwner(ctx, op.Path)
	if err != nil {
		if target.IsNotExist(err) {
			// file missing -> expected drift, copyFileOp will create it
			return spec.CheckUnsatisfied, nil
		}

		// non-transient error (perm, IO, etc.) -> abort
		return spec.CheckUnsatisfied, ownerReadError{
			Path:   op.Path,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	if have.User != op.Owner || have.Group != op.Group {
		return spec.CheckUnsatisfied, nil
	}

	return spec.CheckSatisfied, nil
}

func (op *EnsureOwnerOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	have, err := tgt.GetOwner(ctx, op.Path)
	if err != nil {
		if target.IsNotExist(err) {
			// file should exist - copyFileOp is a dependency and should have created it
			panic(util.BUG("ensureOwnerOp.Execute: file %q does not exist after copyFileOp", op.Path))
		}

		return spec.Result{}, ownerReadError{
			Path:   op.Path,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	changed := have.User != op.Owner || have.Group != op.Group

	if err := tgt.Chown(ctx, op.Path, target.Owner{User: op.Owner, Group: op.Group}); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: changed}, nil
}

type ensureOwnerDesc struct {
	User  string
	Group string
	Path  string
}

func (d ensureOwnerDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   "builtin.ensure-owner",
		Text: `ensure owner "{{.User}}:{{.Group}}" on "{{.Path}}"`,
		Data: d,
	}
}

func (op *EnsureOwnerOp) OpDescription() spec.OpDescription {
	return ensureOwnerDesc{
		User:  op.Owner,
		Group: op.Group,
		Path:  op.Path,
	}
}

type ownerReadError struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e ownerReadError) Error() string {
	return fmt.Sprintf("cannot read ownership of %q: %v", e.Path, e.Err)
}

func (e ownerReadError) Unwrap() error {
	return e.Err
}

func (e ownerReadError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.copy.OwnerReadError",
		Text:   `cannot read ownership of "{{.Path}}"`,
		Hint:   "check file permissions and ensure the path is accessible",
		Data:   e,
		Source: &e.Source,
	}
}

func (ownerReadError) Severity() signal.Severity { return signal.Error }
func (ownerReadError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
