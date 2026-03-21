// SPDX-License-Identifier: GPL-3.0-only

package fileops

import (
	"context"
	"fmt"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const ensureOwnerID = "builtin.ensure-owner"

type EnsureOwnerOp struct {
	sharedops.BaseOp
	Path      string
	Owner     string
	Group     string
	Recursive bool
	OwnerSpan spec.SourceSpan
	GroupSpan spec.SourceSpan
}

func (op *EnsureOwnerOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	owTgt := target.Must[target.Ownership](ensureOwnerID, tgt)
	desired := op.Owner + ":" + op.Group

	if !owTgt.HasUser(ctx, op.Owner) {
		return spec.CheckUnsatisfied, nil, sharedops.UnknownUserError{
			User:   op.Owner,
			Source: op.OwnerSpan,
			Err:    nil,
		}
	}
	if !owTgt.HasGroup(ctx, op.Group) {
		return spec.CheckUnsatisfied, nil, sharedops.UnknownGroupError{
			Group:  op.Group,
			Source: op.GroupSpan,
			Err:    nil,
		}
	}

	res, drift, err := op.checkPath(ctx, owTgt, op.Path, desired)
	if res != spec.CheckSatisfied || err != nil {
		return res, drift, err
	}

	if op.Recursive {
		return op.checkTree(ctx, tgt, op.Path, desired)
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *EnsureOwnerOp) checkPath(
	ctx context.Context,
	owTgt target.Ownership,
	path, desired string,
) (spec.CheckResult, []spec.DriftDetail, error) {
	have, err := owTgt.GetOwner(ctx, path)
	if err != nil {
		if target.IsNotExist(err) {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "owner:group",
				Desired: desired,
			}}, nil
		}

		return spec.CheckUnsatisfied, nil, ownerReadError{
			Path:   path,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	if have.User != op.Owner || have.Group != op.Group {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "owner:group",
			Current: have.User + ":" + have.Group,
			Desired: desired,
		}}, nil
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *EnsureOwnerOp) checkTree(
	ctx context.Context,
	tgt target.Target,
	dir, desired string,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fsTgt := target.Must[target.Filesystem](ensureOwnerID, tgt)
	owTgt := target.Must[target.Ownership](ensureOwnerID, tgt)

	entries, err := fsTgt.ReadDir(ctx, dir)
	if err != nil {
		return spec.CheckUnsatisfied, nil, ownerReadError{
			Path:   dir,
			Err:    err,
			Source: op.DestSpan,
		}
	}

	for _, entry := range entries {
		child := dir + "/" + entry.Name()
		res, drift, err := op.checkPath(ctx, owTgt, child, desired)
		if res != spec.CheckSatisfied || err != nil {
			return res, drift, err
		}
		if entry.IsDir() {
			res, drift, err = op.checkTree(ctx, tgt, child, desired)
			if res != spec.CheckSatisfied || err != nil {
				return res, drift, err
			}
		}
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *EnsureOwnerOp) Execute(ctx context.Context, _ source.Source, tgt target.Target) (spec.Result, error) {
	if op.Recursive {
		return op.executeRecursive(ctx, tgt)
	}

	owTgt := target.Must[target.Ownership](ensureOwnerID, tgt)

	have, err := owTgt.GetOwner(ctx, op.Path)
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

	if err := owTgt.Chown(ctx, op.Path, target.Owner{User: op.Owner, Group: op.Group}); err != nil {
		if target.IsUnknownUser(err) {
			return spec.Result{}, sharedops.UnknownUserError{
				User:   op.Owner,
				Source: op.OwnerSpan,
				Err:    err,
			}
		}
		if target.IsUnknownGroup(err) {
			return spec.Result{}, sharedops.UnknownGroupError{
				Group:  op.Group,
				Source: op.GroupSpan,
				Err:    err,
			}
		}
		// Can't catch during Check: file may not exist yet, and probing
		// write-permission would mutate state in a read-only phase.
		if target.IsPermission(err) {
			return spec.Result{}, sharedops.PermissionDeniedError{
				Operation: fmt.Sprintf("chown %s:%s %s", op.Owner, op.Group, op.Path),
				Source:    op.OwnerSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: changed}, nil
}

func (op *EnsureOwnerOp) executeRecursive(ctx context.Context, tgt target.Target) (spec.Result, error) {
	owTgt := target.Must[target.Ownership](ensureOwnerID, tgt)

	if err := owTgt.ChownRecursive(ctx, op.Path, target.Owner{User: op.Owner, Group: op.Group}); err != nil {
		if target.IsUnknownUser(err) {
			return spec.Result{}, sharedops.UnknownUserError{
				User:   op.Owner,
				Source: op.OwnerSpan,
				Err:    err,
			}
		}
		if target.IsUnknownGroup(err) {
			return spec.Result{}, sharedops.UnknownGroupError{
				Group:  op.Group,
				Source: op.GroupSpan,
				Err:    err,
			}
		}
		if target.IsPermission(err) {
			return spec.Result{}, sharedops.PermissionDeniedError{
				Operation: fmt.Sprintf("chown -R %s:%s %s", op.Owner, op.Group, op.Path),
				Source:    op.OwnerSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: true}, nil
}

func (op EnsureOwnerOp) RequiredCapabilities() capability.Capability {
	return capability.Ownership
}

type ensureOwnerDesc struct {
	User  string
	Group string
	Path  string
}

func (d ensureOwnerDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureOwnerID,
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
	diagnostic.FatalError
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
		ID:     "builtin.OwnerRead",
		Text:   `cannot read ownership of "{{.Path}}"`,
		Hint:   "check file permissions and ensure the path is accessible",
		Data:   e,
		Source: &e.Source,
	}
}
