// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"context"
	"fmt"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const installKeyID = "step.install-repo-key"

type installKeyOp struct {
	sharedop.BaseOp
	source spec.PkgSourceRef
}

func (op *installKeyOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	rm := target.Must[target.RepoManager](installKeyID, tgt)

	has, err := rm.HasRepoKey(ctx, op.source.Name)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}
	if has {
		return spec.CheckSatisfied, nil, nil
	}
	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "signing_key",
		Current: "absent",
		Desired: "present",
	}}, nil
}

func (op *installKeyOp) Execute(
	ctx context.Context,
	src source.Source,
	tgt target.Target,
) (spec.Result, error) {
	rm := target.Must[target.RepoManager](installKeyID, tgt)

	cachePath := keyCachePath(op.source.KeyURL)
	keyData, err := src.ReadFile(ctx, cachePath)
	if err != nil {
		return spec.Result{}, RepoKeyInstallError{
			Name:   op.source.Name,
			Detail: fmt.Sprintf("reading cached key: %v", err),
		}
	}

	keyPath := rm.RepoKeyPath(op.source.Name)
	cfg := target.RepoConfig{
		Name:    op.source.Name,
		KeyData: keyData,
		KeyPath: keyPath,
	}

	if err := rm.InstallRepoKey(ctx, cfg); err != nil {
		return spec.Result{}, RepoKeyInstallError{
			Name:   op.source.Name,
			Detail: err.Error(),
		}
	}

	return spec.Result{Changed: true}, nil
}

func (installKeyOp) RequiredCapabilities() capability.Capability {
	return capability.PkgRepo
}

type installKeyDesc struct {
	Name string
}

func (d installKeyDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   installKeyID,
		Text: `install signing key for "{{.Name}}"`,
		Data: d,
	}
}

func (op *installKeyOp) OpDescription() spec.OpDescription {
	return installKeyDesc{Name: op.source.Name}
}
