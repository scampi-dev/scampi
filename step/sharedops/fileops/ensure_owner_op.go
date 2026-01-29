package fileops

import (
	"context"
	"fmt"

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

type EnsureOwnerOp struct {
	sharedops.BaseOp
	Path      string
	Owner     string
	Group     string
	OwnerSpan spec.SourceSpan
	GroupSpan spec.SourceSpan
}

func (op *EnsureOwnerOp) Check(ctx context.Context, _ source.Source, tgt target.Target) (spec.CheckResult, error) {
	if !tgt.HasUser(ctx, op.Owner) {
		return spec.CheckUnsatisfied, unknownUserError{
			User:   op.Owner,
			Source: op.OwnerSpan,
			Err:    nil,
		}
	}
	if !tgt.HasGroup(ctx, op.Group) {
		return spec.CheckUnsatisfied, unknownGroupError{
			Group:  op.Group,
			Source: op.GroupSpan,
			Err:    nil,
		}
	}

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
			panic(errs.BUG("ensureOwnerOp.Execute: file %q does not exist after copyFileOp", op.Path))
		}

		return spec.Result{}, ownerReadError{
			Path:   op.Path,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	changed := have.User != op.Owner || have.Group != op.Group

	if err := tgt.Chown(ctx, op.Path, target.Owner{User: op.Owner, Group: op.Group}); err != nil {
		if target.IsUnknownUser(err) {
			return spec.Result{}, unknownUserError{
				User:   op.Owner,
				Source: op.OwnerSpan,
				Err:    err,
			}
		}
		return spec.Result{}, err
	}

	return spec.Result{Changed: changed}, nil
}

func (EnsureOwnerOp) RequiredCapabilities() capability.Capability {
	return capability.Ownership
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

type unknownUserError struct {
	User   string
	Source spec.SourceSpan
	Err    error
}

func (e unknownUserError) Error() string {
	return fmt.Sprintf("unknown user %q: %v", e.User, e.Err)
}

func (e unknownUserError) Unwrap() error {
	return e.Err
}

func (e unknownUserError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.unknownUserError",
		Text:   `unknown user "{{.User}}"`,
		Hint:   "create user before changing file owner",
		Data:   e,
		Source: &e.Source,
	}
}

func (unknownUserError) Severity() signal.Severity { return signal.Error }
func (unknownUserError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type unknownGroupError struct {
	Group  string
	Source spec.SourceSpan
	Err    error
}

func (e unknownGroupError) Error() string {
	return fmt.Sprintf("unknown group %q: %v", e.Group, e.Err)
}

func (e unknownGroupError) Unwrap() error {
	return e.Err
}

func (e unknownGroupError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.unknownGroupError",
		Text:   `unknown group "{{.Group}}"`,
		Hint:   "create group before changing file owner",
		Data:   e,
		Source: &e.Source,
	}
}

func (unknownGroupError) Severity() signal.Severity { return signal.Error }
func (unknownGroupError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
