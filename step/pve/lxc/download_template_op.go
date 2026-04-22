// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"fmt"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const downloadTemplateID = "step.pve.lxc.download-template"

type downloadTemplateOp struct {
	sharedops.BaseOp
	template LxcTemplate
	step     spec.StepInstance
}

func (op *downloadTemplateOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](downloadTemplateID, tgt)

	result, err := cmdr.RunPrivileged(ctx, fmt.Sprintf("pveam list %s", op.template.Storage))
	if err != nil {
		return spec.CheckUnsatisfied, nil, op.cmdErrWrap(err)
	}
	if result.ExitCode != 0 {
		return spec.CheckUnsatisfied, nil, op.cmdErrStr(result.Stderr)
	}

	if strings.Contains(result.Stdout, op.template.Name) {
		return spec.CheckSatisfied, nil, nil
	}

	// Not on storage — verify it's available for download.
	result, err = cmdr.RunPrivileged(ctx, "pveam available")
	if err != nil {
		return spec.CheckUnsatisfied, nil, op.cmdErrWrap(err)
	}
	if result.ExitCode != 0 {
		return spec.CheckUnsatisfied, nil, op.cmdErrStr(result.Stderr)
	}

	if !strings.Contains(result.Stdout, op.template.Name) {
		return spec.CheckUnsatisfied, nil, TemplateNotFoundError{
			Template: op.template.Name,
			Storage:  op.template.Storage,
			Source:   op.step.Fields["template"].Value,
		}
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "template",
		Current: "(absent)",
		Desired: op.template.templatePath(),
	}}, nil
}

func (op *downloadTemplateOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](downloadTemplateID, tgt)

	cmd := buildDownloadCmd(op.template.Storage, op.template.Name)
	result, err := cmdr.RunPrivileged(ctx, cmd)
	if err != nil {
		return spec.Result{}, op.cmdErrWrap(err)
	}
	if result.ExitCode != 0 {
		return spec.Result{}, op.cmdErrStr(result.Stderr)
	}

	return spec.Result{Changed: true}, nil
}

func (op *downloadTemplateOp) cmdErrWrap(err error) CommandFailedError {
	return CommandFailedError{
		Op:     "pveam download",
		VMID:   0,
		Stderr: err.Error(),
		Source: op.step.Source,
	}
}

func (op *downloadTemplateOp) cmdErrStr(stderr string) CommandFailedError {
	return CommandFailedError{
		Op:     "pveam download",
		VMID:   0,
		Stderr: stderr,
		Source: op.step.Source,
	}
}

func (downloadTemplateOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type downloadTemplateDesc struct {
	Template string
}

func (d downloadTemplateDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   downloadTemplateID,
		Text: `download template {{.Template}}`,
		Data: d,
	}
}

func (op *downloadTemplateOp) OpDescription() spec.OpDescription {
	return downloadTemplateDesc{
		Template: op.template.templatePath(),
	}
}

func (op *downloadTemplateOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "storage", Value: op.template.Storage},
		{Label: "template", Value: op.template.Name},
	}
}
