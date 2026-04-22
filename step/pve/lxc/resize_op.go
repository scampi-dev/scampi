// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"fmt"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const resizeLxcID = "step.pve.lxc.resize"

type resizeLxcOp struct {
	sharedops.BaseOp
	pveCmd
	sizeGiB int
}

func (op *resizeLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](resizeLxcID, tgt)

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}
	if !exists {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "size",
			Current: "(pending create)",
			Desired: fmt.Sprintf("%dG", op.sizeGiB),
		}}, nil
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	// Shrink check.
	if cfg.Size != "" {
		currentGiB := parseSizeGiB(cfg.Size)
		if op.sizeGiB > 0 && currentGiB > 0 && op.sizeGiB < currentGiB {
			return spec.CheckUnsatisfied, nil, ResizeShrinkError{
				Current: fmt.Sprintf("%dG", currentGiB),
				Desired: fmt.Sprintf("%dG", op.sizeGiB),
				Source:  op.step.Fields["size"].Value,
			}
		}
		if op.sizeGiB > currentGiB {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "size",
				Current: fmt.Sprintf("%dG", currentGiB),
				Desired: fmt.Sprintf("%dG", op.sizeGiB),
			}}, nil
		}
	}

	return spec.CheckSatisfied, nil, nil
}

func (op *resizeLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](resizeLxcID, tgt)

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil || !exists {
		return spec.Result{}, err
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	if cfg.Size == "" {
		return spec.Result{}, nil
	}

	currentGiB := parseSizeGiB(cfg.Size)
	if op.sizeGiB <= currentGiB {
		return spec.Result{}, nil
	}

	cmd := fmt.Sprintf("pct resize %d rootfs %dG", op.id, op.sizeGiB)
	if err := op.runCmd(ctx, cmdr, "resize", cmd); err != nil {
		return spec.Result{}, err
	}

	return spec.Result{Changed: true}, nil
}

func (resizeLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type resizeLxcDesc struct {
	VMID int
	Size string
}

func (d resizeLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   resizeLxcID,
		Text: `resize LXC {{.VMID}} rootfs to {{.Size}}`,
		Data: d,
	}
}

func (op *resizeLxcOp) OpDescription() spec.OpDescription {
	return resizeLxcDesc{
		VMID: op.id,
		Size: fmt.Sprintf("%dG", op.sizeGiB),
	}
}

func (op *resizeLxcOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "vmid", Value: fmt.Sprintf("%d", op.id)},
		{Label: "size", Value: fmt.Sprintf("%dG", op.sizeGiB)},
	}
}
