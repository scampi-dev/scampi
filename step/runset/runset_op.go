// SPDX-License-Identifier: GPL-3.0-only

package runset

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

const runSetID = "step.run_set"

type runSetOp struct {
	sharedop.BaseOp
	list      string
	addTpl    *itemTemplate
	removeTpl *itemTemplate
	desired   []string
	init      string
	env       map[string]string
	source    spec.SourceSpan

	// Set during Check, consumed during Execute.
	plan setPlan
}

// setPlan is the result of diffing live against desired.
type setPlan struct {
	live     []string // sorted, deduped, post-init
	toAdd    []string // desired - live, in desired order
	toRemove []string // live - desired, sorted (live order is non-deterministic)
}

func (op *runSetOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](runSetID, tgt)

	live, err := op.listLive(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	plan := diff(live, op.desired, op.addTpl != nil, op.removeTpl != nil)
	op.plan = plan

	if len(plan.toAdd) == 0 && len(plan.toRemove) == 0 {
		return spec.CheckSatisfied, nil, nil
	}

	drift := make([]spec.DriftDetail, 0, len(plan.toAdd)+len(plan.toRemove))
	for _, item := range plan.toAdd {
		drift = append(drift, spec.DriftDetail{
			Field: item, Current: "missing", Desired: "present",
		})
	}
	for _, item := range plan.toRemove {
		drift = append(drift, spec.DriftDetail{
			Field: item, Current: "present", Desired: "absent",
		})
	}
	return spec.CheckUnsatisfied, drift, nil
}

func (op *runSetOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](runSetID, tgt)

	changed := false
	if len(op.plan.toRemove) > 0 && op.removeTpl != nil {
		for _, cmd := range op.removeTpl.render(op.plan.toRemove) {
			res, err := cmdr.RunCommand(ctx, op.withEnv(cmd))
			if err != nil {
				return spec.Result{}, RemoveFailedError{
					Cmd: cmd, Stderr: err.Error(), Source: op.source,
				}
			}
			if res.ExitCode != 0 {
				return spec.Result{}, RemoveFailedError{
					Cmd: cmd, ExitCode: res.ExitCode, Stderr: res.Stderr, Source: op.source,
				}
			}
		}
		changed = true
	}
	if len(op.plan.toAdd) > 0 && op.addTpl != nil {
		for _, cmd := range op.addTpl.render(op.plan.toAdd) {
			res, err := cmdr.RunCommand(ctx, op.withEnv(cmd))
			if err != nil {
				return spec.Result{}, AddFailedError{
					Cmd: cmd, Stderr: err.Error(), Source: op.source,
				}
			}
			if res.ExitCode != 0 {
				return spec.Result{}, AddFailedError{
					Cmd: cmd, ExitCode: res.ExitCode, Stderr: res.Stderr, Source: op.source,
				}
			}
		}
		changed = true
	}
	return spec.Result{Changed: changed}, nil
}

// listLive runs the list command, optionally bootstrapping with init
// when list exits non-zero. Returns the parsed identifier set.
func (op *runSetOp) listLive(ctx context.Context, cmdr target.Command) ([]string, error) {
	res, err := cmdr.RunCommand(ctx, op.withEnv(op.list))
	if err != nil {
		return nil, ListFailedError{Cmd: op.list, Stderr: err.Error(), Source: op.source}
	}
	if res.ExitCode != 0 {
		if op.init == "" {
			return nil, ListFailedError{
				Cmd: op.list, ExitCode: res.ExitCode, Stderr: res.Stderr, Source: op.source,
			}
		}
		// init bootstrap: run, then re-list.
		initRes, err := cmdr.RunCommand(ctx, op.withEnv(op.init))
		if err != nil {
			return nil, InitFailedError{Cmd: op.init, Stderr: err.Error(), Source: op.source}
		}
		if initRes.ExitCode != 0 {
			return nil, InitFailedError{
				Cmd: op.init, ExitCode: initRes.ExitCode, Stderr: initRes.Stderr, Source: op.source,
			}
		}
		res, err = cmdr.RunCommand(ctx, op.withEnv(op.list))
		if err != nil {
			return nil, ListFailedError{Cmd: op.list, Stderr: err.Error(), Source: op.source}
		}
		if res.ExitCode != 0 {
			return nil, ListFailedError{
				Cmd: op.list, ExitCode: res.ExitCode, Stderr: res.Stderr, Source: op.source,
			}
		}
	}
	return parseListStdout(res.Stdout), nil
}

// parseListStdout splits stdout on newlines, trims whitespace, drops
// empty lines, and dedupes.
func parseListStdout(stdout string) []string {
	if stdout == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for line := range strings.SplitSeq(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, dup := seen[line]; dup {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	return out
}

// diff computes the add/remove sets given live and desired identifier
// lists. addEnabled and removeEnabled gate each side: if the user did
// not declare an `add` template, missing items are not flagged as
// drift (and vice versa) — that's how you get one-way reconciliation
// (e.g. "manage adds, leave orphans alone").
func diff(live, desired []string, addEnabled, removeEnabled bool) setPlan {
	liveSet := toSet(live)
	desiredSet := toSet(desired)

	plan := setPlan{live: live}
	if addEnabled {
		for _, d := range desired {
			if _, in := liveSet[d]; !in {
				plan.toAdd = append(plan.toAdd, d)
			}
		}
	}
	if removeEnabled {
		var orphans []string
		for l := range liveSet {
			if _, in := desiredSet[l]; !in {
				orphans = append(orphans, l)
			}
		}
		sort.Strings(orphans)
		plan.toRemove = orphans
	}
	return plan
}

func toSet(xs []string) map[string]struct{} {
	out := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		out[x] = struct{}{}
	}
	return out
}

// envPrefix builds a deterministic `KEY1='v1' KEY2='v2' ` prefix.
// Mirrors step/run.envPrefix; the two are duplicated rather than
// shared to avoid pulling step/run as a runset dep.
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

func (op *runSetOp) withEnv(cmd string) string {
	if prefix := envPrefix(op.env); prefix != "" {
		return prefix + cmd
	}
	return cmd
}

func (runSetOp) RequiredCapabilities() capability.Capability {
	return capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type runSetOpDesc struct {
	Desc    string
	List    string
	Desired int
}

func (d runSetOpDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   runSetID,
		Text: `{{if .Desc}}{{.Desc}}{{else}}run_set list={{.List}} ({{.Desired}} desired){{end}}`,
		Data: d,
	}
}

func (op *runSetOp) OpDescription() spec.OpDescription {
	return runSetOpDesc{
		Desc:    op.Action().Desc(),
		List:    op.list,
		Desired: len(op.desired),
	}
}

func (op *runSetOp) Inspect() []spec.InspectField {
	fields := []spec.InspectField{
		{Label: "list", Value: op.list},
		{Label: "desired", Value: fmt.Sprintf("[%s]", strings.Join(op.desired, ", "))},
	}
	if op.addTpl != nil {
		fields = append(fields, spec.InspectField{Label: "add", Value: op.addTpl.raw})
	}
	if op.removeTpl != nil {
		fields = append(fields, spec.InspectField{Label: "remove", Value: op.removeTpl.raw})
	}
	if op.init != "" {
		fields = append(fields, spec.InspectField{Label: "init", Value: op.init})
	}
	return fields
}
