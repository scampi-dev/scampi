// SPDX-License-Identifier: GPL-3.0-only

package sysctl

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

const setSysctlID = "builtin.sysctl.set"

type setSysctlOp struct {
	sharedops.BaseOp
	key   string
	value string
}

func (op *setSysctlOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](setSysctlID, tgt)

	result, err := cmdr.RunCommand(ctx, fmt.Sprintf("sysctl -n %s", op.key))
	if err != nil {
		return spec.CheckUnsatisfied, nil, sharedops.DiagnoseTargetError(err)
	}
	if result.ExitCode != 0 {
		stderr := result.Stderr
		if stderr == "" {
			stderr = fmt.Sprintf("exit %d", result.ExitCode)
		}
		return spec.CheckUnsatisfied, nil, ReadError{
			Key:    op.key,
			Stderr: stderr,
		}
	}

	current := strings.TrimSpace(result.Stdout)
	if current == op.value {
		return spec.CheckSatisfied, nil, nil
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   op.key,
		Current: current,
		Desired: op.value,
	}}, nil
}

func (op *setSysctlOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](setSysctlID, tgt)

	result, err := cmdr.RunCommand(ctx, fmt.Sprintf("sysctl -w %s=%s", op.key, op.value))
	if err != nil {
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}
	if result.ExitCode != 0 {
		stderr := result.Stderr
		if stderr == "" {
			stderr = fmt.Sprintf("exit %d", result.ExitCode)
		}
		return spec.Result{}, WriteError{
			Key:    op.key,
			Value:  op.value,
			Stderr: stderr,
		}
	}

	return spec.Result{Changed: true}, nil
}

func (setSysctlOp) RequiredCapabilities() capability.Capability {
	return capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type setSysctlDesc struct {
	Key   string
	Value string
}

func (d setSysctlDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   setSysctlID,
		Text: `set sysctl {{.Key}} = {{.Value}}`,
		Data: d,
	}
}

func (op *setSysctlOp) OpDescription() spec.OpDescription {
	return setSysctlDesc{Key: op.key, Value: op.value}
}
