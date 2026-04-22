// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const configLxcID = "step.pve.lxc.config"

type configLxcOp struct {
	sharedops.BaseOp
	pveCmd
	node       string
	hostname   string
	cores      int
	memoryMiB  int
	swapMiB    int
	storage    string
	privileged bool
	network    LxcNet
	tags       []string
}

func (op *configLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](configLxcID, tgt)

	if err := op.checkNode(ctx, cmdr, op.node); err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}
	if !exists {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "config",
			Current: "(pending create)",
			Desired: "configured",
		}}, nil
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	if err := op.checkImmutables(cfg); err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	drift := op.configDrift(cfg)
	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
}

func (op *configLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](configLxcID, tgt)

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil || !exists {
		return spec.Result{}, err
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	changed := false

	drift := op.configDrift(cfg)
	setDrift := filterSetDrift(drift)
	if len(setDrift) > 0 {
		if err := op.runCmd(ctx, cmdr, "set", buildSetCmd(op.id, setDrift)); err != nil {
			return spec.Result{}, err
		}
		changed = true
	}

	if hasNetworkDrift(drift) {
		cmd := fmt.Sprintf("pct set %d --net0 %s", op.id, formatNet0(op.network))
		if err := op.runCmd(ctx, cmdr, "set network", cmd); err != nil {
			return spec.Result{}, err
		}
		changed = true
	}

	return spec.Result{Changed: changed}, nil
}

func (op *configLxcOp) checkImmutables(cfg pctConfig) error {
	wantUnpriv := boolToInt(!op.privileged)
	if cfg.Unprivileged != wantUnpriv {
		return ImmutableFieldError{
			Field:   "privileged",
			Current: strconv.FormatBool(cfg.Unprivileged == 0),
			Desired: strconv.FormatBool(op.privileged),
			Source:  op.step.Fields["privileged"].Value,
		}
	}

	if cfg.Storage != "" && cfg.Storage != op.storage {
		return ImmutableFieldError{
			Field:   "storage",
			Current: cfg.Storage,
			Desired: op.storage,
			Source:  op.step.Fields["storage"].Value,
		}
	}

	return nil
}

func (op *configLxcOp) configDrift(cfg pctConfig) []spec.DriftDetail {
	var drift []spec.DriftDetail

	if cfg.Cores != 0 && cfg.Cores != op.cores {
		drift = append(drift, spec.DriftDetail{
			Field:   "cores",
			Current: strconv.Itoa(cfg.Cores),
			Desired: strconv.Itoa(op.cores),
		})
	}
	if cfg.Memory != 0 && cfg.Memory != op.memoryMiB {
		drift = append(drift, spec.DriftDetail{
			Field:   "memory",
			Current: strconv.Itoa(cfg.Memory),
			Desired: strconv.Itoa(op.memoryMiB),
		})
	}
	if cfg.Swap != 0 && cfg.Swap != op.swapMiB {
		drift = append(drift, spec.DriftDetail{
			Field:   "swap",
			Current: strconv.Itoa(cfg.Swap),
			Desired: strconv.Itoa(op.swapMiB),
		})
	}
	if cfg.Hostname != "" && cfg.Hostname != op.hostname {
		drift = append(drift, spec.DriftDetail{
			Field:   "hostname",
			Current: cfg.Hostname,
			Desired: op.hostname,
		})
	}
	if cfg.Description != op.step.Desc {
		drift = append(drift, spec.DriftDetail{
			Field:   "description",
			Current: valueOrNone(cfg.Description),
			Desired: valueOrNone(op.step.Desc),
		})
	}

	desiredTags := strings.Join(op.tags, ";")
	if cfg.Tags != desiredTags {
		drift = append(drift, spec.DriftDetail{
			Field:   "tags",
			Current: valueOrNone(cfg.Tags),
			Desired: valueOrNone(desiredTags),
		})
	}

	if cfg.Net.Bridge != "" {
		bridge := op.network.Bridge
		if bridge == "" {
			bridge = "vmbr0"
		}
		if cfg.Net.Bridge != bridge {
			drift = append(drift, spec.DriftDetail{
				Field:   "network.bridge",
				Current: cfg.Net.Bridge,
				Desired: bridge,
			})
		}
	}
	if cfg.Net.IP != "" && cfg.Net.IP != op.network.IP {
		drift = append(drift, spec.DriftDetail{
			Field:   "network.ip",
			Current: cfg.Net.IP,
			Desired: op.network.IP,
		})
	}
	if cfg.Net.Gw != op.network.Gw {
		if cfg.Net.Gw != "" || op.network.Gw != "" {
			drift = append(drift, spec.DriftDetail{
				Field:   "network.gw",
				Current: valueOrNone(cfg.Net.Gw),
				Desired: valueOrNone(op.network.Gw),
			})
		}
	}

	return drift
}

func (configLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type configLxcDesc struct {
	VMID int
}

func (d configLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   configLxcID,
		Text: `configure LXC {{.VMID}}`,
		Data: d,
	}
}

func (op *configLxcOp) OpDescription() spec.OpDescription {
	return configLxcDesc{VMID: op.id}
}

func (op *configLxcOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "vmid", Value: fmt.Sprintf("%d", op.id)},
		{Label: "hostname", Value: op.hostname},
		{Label: "cores", Value: fmt.Sprintf("%d", op.cores)},
		{Label: "memory", Value: fmt.Sprintf("%d MiB", op.memoryMiB)},
		{Label: "swap", Value: fmt.Sprintf("%d MiB", op.swapMiB)},
		{Label: "network", Value: formatNet0(op.network)},
		{Label: "tags", Value: strings.Join(op.tags, ", ")},
	}
}
