// SPDX-License-Identifier: GPL-3.0-only

package run

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const runID = "step.run"

type runOp struct {
	sharedop.BaseOp
	apply  string
	check  string
	always bool
	env    map[string]string
}

// envPrefix builds a deterministic `KEY1='v1' KEY2='v2' ` prefix for
// the env map. Values are shell-quoted so spaces / quotes / shell
// metacharacters pass through unmolested. Keys are sorted so the
// generated command is stable across runs (debuggable, diffable).
//
// Empty map → empty prefix (no overhead for the zero-env case).
func envPrefix(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(target.ShellQuote(env[k]))
		b.WriteByte(' ')
	}
	return b.String()
}

// withEnv prepends the env prefix to cmd. Returns cmd unchanged when
// env is empty so existing call sites stay byte-identical.
func (op *runOp) withEnv(cmd string) string {
	if prefix := envPrefix(op.env); prefix != "" {
		return prefix + cmd
	}
	return cmd
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
	result, err := cmdr.RunCommand(ctx, op.withEnv(op.check))
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

	result, err := cmdr.RunCommand(ctx, op.withEnv(op.apply))
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
		verify, err := cmdr.RunCommand(ctx, op.withEnv(op.check))
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

func (op *runOp) Inspect() []spec.InspectField {
	fields := []spec.InspectField{
		{Label: "apply", Value: op.apply},
	}
	if op.check != "" {
		fields = append(fields, spec.InspectField{Label: "check", Value: op.check})
	}
	if op.always {
		fields = append(fields, spec.InspectField{Label: "always", Value: "true"})
	}
	return fields
}
