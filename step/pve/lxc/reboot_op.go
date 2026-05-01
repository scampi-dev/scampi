// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"fmt"
	"time"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const rebootLxcID = "step.pve.lxc.reboot"

// rebootCheck probes the running container for a field that needs a reboot
// to take effect.
type rebootCheck struct {
	field   string
	desired string
	probe   func(ctx context.Context, cmdr target.Command, id int) string
}

// rebootAware is implemented by ops whose changes require a container
// reboot to take effect. The reboot op collects checks from all ops
// at construction time — adding reboot awareness to a new op is just
// implementing this interface.
type rebootAware interface {
	RebootChecks() []rebootCheck
}

type rebootLxcOp struct {
	sharedop.BaseOp
	pveCmd
	checks []rebootCheck
}

func (op *rebootLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](rebootLxcID, tgt)
	return op.checkWith(ctx, cmdr)
}

func (op *rebootLxcOp) checkWith(
	ctx context.Context,
	cmdr target.Command,
) (spec.CheckResult, []spec.DriftDetail, error) {
	result, err := cmdr.RunPrivileged(ctx, fmt.Sprintf("pct status %d", op.id))
	if err != nil || result.ExitCode != 0 {
		return spec.CheckSatisfied, nil, nil
	}
	if parsePctStatus(result.Stdout) != stateRunning {
		return spec.CheckSatisfied, nil, nil
	}

	var drift []spec.DriftDetail
	for _, check := range op.checks {
		current := check.probe(ctx, cmdr, op.id)
		if current == "" || current == check.desired {
			continue
		}
		drift = append(drift, spec.DriftDetail{
			Field:   check.field + " (reboot)",
			Current: current,
			Desired: check.desired,
		})
	}

	if len(drift) == 0 {
		return spec.CheckSatisfied, nil, nil
	}
	return spec.CheckUnsatisfied, drift, nil
}

func (op *rebootLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](rebootLxcID, tgt)

	result, err := cmdr.RunPrivileged(ctx, fmt.Sprintf("pct status %d", op.id))
	if err == nil && parsePctStatus(result.Stdout) != stateRunning {
		return spec.Result{}, nil
	}

	cmd := fmt.Sprintf("pct reboot %d --timeout 30", op.id)
	if err := op.runCmd(ctx, cmdr, "reboot", cmd); err != nil {
		return spec.Result{}, err
	}

	if err := op.waitRunning(ctx, cmdr); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}

func (op *rebootLxcOp) waitRunning(
	ctx context.Context,
	cmdr target.Command,
) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return op.cmdErr("reboot", "container did not come back within 60s")
		case <-ticker.C:
			r, err := cmdr.RunPrivileged(ctx, fmt.Sprintf("pct status %d", op.id))
			if err != nil || r.ExitCode != 0 {
				continue
			}
			if parsePctStatus(r.Stdout) == stateRunning {
				return nil
			}
		}
	}
}

func (rebootLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type rebootLxcDesc struct {
	VMID int
}

func (d rebootLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   rebootLxcID,
		Text: `reboot LXC {{.VMID}} if needed`,
		Data: d,
	}
}

func (op *rebootLxcOp) OpDescription() spec.OpDescription {
	return rebootLxcDesc{VMID: op.id}
}

func (op *rebootLxcOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "vmid", Value: fmt.Sprintf("%d", op.id)},
	}
}
