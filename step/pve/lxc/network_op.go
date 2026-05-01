// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"fmt"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const networkLxcID = "step.pve.lxc.network"

type networkLxcOp struct {
	sharedop.BaseOp
	pveCmd
	networks []LxcNet
}

func (op *networkLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](networkLxcID, tgt)

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}
	if !exists {
		if len(op.networks) > 0 {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "networks",
				Current: "(pending create)",
				Desired: fmt.Sprintf("%d NIC(s)", len(op.networks)),
			}}, nil
		}
		return spec.CheckSatisfied, nil, nil
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	drift := op.networkDrift(cfg)
	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
}

func (op *networkLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](networkLxcID, tgt)

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil || !exists {
		return spec.Result{}, err
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	drift := op.networkDrift(cfg)
	if len(drift) == 0 {
		return spec.Result{}, nil
	}

	if err := op.applyNetworkDrift(ctx, cmdr, cfg); err != nil {
		return spec.Result{}, err
	}
	return spec.Result{Changed: true}, nil
}

func (op *networkLxcOp) networkDrift(cfg pctConfig) []spec.DriftDetail {
	var drift []spec.DriftDetail
	maxNets := max(len(cfg.Nets), len(op.networks))
	for i := range maxNets {
		field := fmt.Sprintf("network[%d]", i)
		if i >= len(op.networks) {
			drift = append(drift, spec.DriftDetail{
				Field:   field,
				Current: formatNet(i, parsedToLxcNet(cfg.Nets[i])),
				Desired: "(absent)",
			})
			continue
		}
		if i >= len(cfg.Nets) {
			drift = append(drift, spec.DriftDetail{
				Field:   field,
				Current: "(absent)",
				Desired: formatNet(i, op.networks[i]),
			})
			continue
		}
		desired := formatNet(i, op.networks[i])
		current := formatNet(i, parsedToLxcNet(cfg.Nets[i]))
		if current != desired {
			drift = append(drift, spec.DriftDetail{
				Field:   field,
				Current: current,
				Desired: desired,
			})
		}
	}
	return drift
}

func (op *networkLxcOp) applyNetworkDrift(
	ctx context.Context,
	cmdr target.Command,
	cfg pctConfig,
) error {
	maxNets := max(len(cfg.Nets), len(op.networks))

	for i := maxNets - 1; i >= 0; i-- {
		if i >= len(cfg.Nets) {
			continue
		}
		if i < len(op.networks) {
			desired := formatNet(i, op.networks[i])
			current := formatNet(i, parsedToLxcNet(cfg.Nets[i]))
			if current == desired {
				continue
			}
		}
		cmd := fmt.Sprintf("pct set %d --delete net%d", op.id, i)
		if err := op.runCmd(ctx, cmdr, "delete network", cmd); err != nil {
			return err
		}
	}

	for i, net := range op.networks {
		if i < len(cfg.Nets) {
			desired := formatNet(i, net)
			current := formatNet(i, parsedToLxcNet(cfg.Nets[i]))
			if current == desired {
				continue
			}
		}
		cmd := fmt.Sprintf("pct set %d --net%d %s", op.id, i, formatNet(i, net))
		if err := op.runCmd(ctx, cmdr, "set network", cmd); err != nil {
			return err
		}
	}
	return nil
}

func (networkLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type networkLxcDesc struct {
	VMID int
	NICs int
}

func (d networkLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   networkLxcID,
		Text: `manage networks for LXC {{.VMID}} ({{.NICs}} NIC(s))`,
		Data: d,
	}
}

func (op *networkLxcOp) OpDescription() spec.OpDescription {
	return networkLxcDesc{VMID: op.id, NICs: len(op.networks)}
}

func (op *networkLxcOp) Inspect() []spec.InspectField {
	var fields []spec.InspectField
	for i, net := range op.networks {
		fields = append(fields, spec.InspectField{
			Label: fmt.Sprintf("net%d", i),
			Value: formatNet(i, net),
		})
	}
	return fields
}
