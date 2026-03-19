// SPDX-License-Identifier: GPL-3.0-only

package run

import (
	"context"
	"fmt"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const runID = "builtin.run"

type runOp struct {
	sharedops.BaseOp
	apply  string
	check  string
	always bool
}

func (op *runOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	if op.always {
		return spec.CheckUnsatisfied, nil, nil
	}

	cmdr := target.Must[target.Command](runID, tgt)
	result, err := cmdr.RunCommand(ctx, op.check)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	if result.ExitCode == 0 {
		return spec.CheckSatisfied, nil, nil
	}

	detail := fmt.Sprintf("exit %d", result.ExitCode)
	if result.Stderr != "" {
		detail = result.Stderr
	}
	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "check",
		Current: detail,
		Desired: "exit 0",
	}}, nil
}

func (op *runOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](runID, tgt)

	result, err := cmdr.RunCommand(ctx, op.apply)
	if err != nil {
		return spec.Result{}, ApplyError{
			Cmd:    op.apply,
			Stderr: err.Error(),
		}
	}
	if result.ExitCode != 0 {
		stderr := result.Stderr
		if stderr == "" {
			stderr = fmt.Sprintf("exit %d", result.ExitCode)
		}
		return spec.Result{}, ApplyError{
			Cmd:    op.apply,
			Stderr: stderr,
		}
	}

	// Re-run check after apply to verify convergence
	if op.check != "" {
		verify, err := cmdr.RunCommand(ctx, op.check)
		if err != nil {
			return spec.Result{}, PostApplyCheckError{
				CheckCmd: op.check,
				ApplyCmd: op.apply,
				Stderr:   err.Error(),
			}
		}
		if verify.ExitCode != 0 {
			stderr := verify.Stderr
			if stderr == "" {
				stderr = fmt.Sprintf("exit %d", verify.ExitCode)
			}
			return spec.Result{}, PostApplyCheckError{
				CheckCmd: op.check,
				ApplyCmd: op.apply,
				Stderr:   stderr,
			}
		}
	}

	return spec.Result{Changed: true}, nil
}

func (runOp) RequiredCapabilities() capability.Capability {
	return capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type runOpDesc struct {
	Apply  string
	Always bool
}

func (d runOpDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   runID,
		Text: `run{{if .Always}} (always){{end}}: {{.Apply}}`,
		Data: d,
	}
}

func (op *runOp) OpDescription() spec.OpDescription {
	return runOpDesc{
		Apply:  op.apply,
		Always: op.always,
	}
}
