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

const deviceLxcID = "step.pve.lxc.device"

type deviceLxcOp struct {
	sharedop.BaseOp
	pveCmd
	devices []LxcDevice
}

func (op *deviceLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](deviceLxcID, tgt)

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}
	if !exists {
		if len(op.devices) > 0 {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "devices",
				Current: "(pending create)",
				Desired: fmt.Sprintf("%d device(s)", len(op.devices)),
			}}, nil
		}
		return spec.CheckSatisfied, nil, nil
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	drift := op.deviceDrift(cfg)
	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
}

func (op *deviceLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](deviceLxcID, tgt)

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil || !exists {
		return spec.Result{}, err
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	drift := op.deviceDrift(cfg)
	if len(drift) == 0 {
		return spec.Result{}, nil
	}

	if err := op.applyDeviceDrift(ctx, cmdr, cfg); err != nil {
		return spec.Result{}, err
	}
	return spec.Result{Changed: true}, nil
}

func (op *deviceLxcOp) deviceDrift(cfg pctConfig) []spec.DriftDetail {
	var drift []spec.DriftDetail
	maxDevs := max(len(cfg.Devs), len(op.devices))
	for i := range maxDevs {
		field := fmt.Sprintf("device[%d]", i)
		if i >= len(op.devices) {
			drift = append(drift, spec.DriftDetail{
				Field:   field,
				Current: formatDev(parsedToLxcDevice(cfg.Devs[i])),
				Desired: "(absent)",
			})
			continue
		}
		if i >= len(cfg.Devs) {
			drift = append(drift, spec.DriftDetail{
				Field:   field,
				Current: "(absent)",
				Desired: formatDev(op.devices[i]),
			})
			continue
		}
		desired := formatDev(op.devices[i])
		current := formatDev(parsedToLxcDevice(cfg.Devs[i]))
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

func (op *deviceLxcOp) applyDeviceDrift(
	ctx context.Context,
	cmdr target.Command,
	cfg pctConfig,
) error {
	maxDevs := max(len(cfg.Devs), len(op.devices))

	for i := maxDevs - 1; i >= 0; i-- {
		if i >= len(cfg.Devs) {
			continue
		}
		if i < len(op.devices) {
			desired := formatDev(op.devices[i])
			current := formatDev(parsedToLxcDevice(cfg.Devs[i]))
			if current == desired {
				continue
			}
		}
		cmd := fmt.Sprintf("pct set %d --delete dev%d", op.id, i)
		if err := op.runCmd(ctx, cmdr, "delete device", cmd); err != nil {
			return err
		}
	}

	for i, dev := range op.devices {
		if i < len(cfg.Devs) {
			desired := formatDev(dev)
			current := formatDev(parsedToLxcDevice(cfg.Devs[i]))
			if current == desired {
				continue
			}
		}
		cmd := fmt.Sprintf("pct set %d --dev%d %s", op.id, i, formatDev(dev))
		if err := op.runCmd(ctx, cmdr, "set device", cmd); err != nil {
			return err
		}
	}
	return nil
}

func (op *deviceLxcOp) RebootChecks() []rebootCheck {
	return []rebootCheck{{
		field:   "devices",
		desired: devicesFingerprint(op.devices),
		probe: func(ctx context.Context, cmdr target.Command, id int) string {
			cfg, err := op.inspectConfig(ctx, cmdr)
			if err != nil {
				return ""
			}
			var live []LxcDevice
			for _, d := range cfg.Devs {
				live = append(live, parsedToLxcDevice(d))
			}
			configFP := devicesFingerprint(live)
			desiredFP := devicesFingerprint(op.devices)
			if configFP != desiredFP {
				return configFP
			}
			for _, d := range op.devices {
				cmd := fmt.Sprintf("pct exec %d -- test -e %s", id, shellQuote(d.Path))
				r, err := cmdr.RunPrivileged(ctx, cmd)
				if err != nil || r.ExitCode != 0 {
					return "stale (device missing)"
				}
			}
			if len(op.devices) == 0 && len(cfg.Devs) == 0 {
				cmd := fmt.Sprintf("pct exec %d -- test -d /dev/dri", id)
				r, err := cmdr.RunPrivileged(ctx, cmd)
				if err == nil && r.ExitCode == 0 {
					return "stale (device remnant)"
				}
			}
			return desiredFP
		},
	}}
}

func (deviceLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type deviceLxcDesc struct {
	VMID    int
	Devices int
}

func (d deviceLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   deviceLxcID,
		Text: `manage devices for LXC {{.VMID}} ({{.Devices}} device(s))`,
		Data: d,
	}
}

func (op *deviceLxcOp) OpDescription() spec.OpDescription {
	return deviceLxcDesc{VMID: op.id, Devices: len(op.devices)}
}

func (op *deviceLxcOp) Inspect() []spec.InspectField {
	var fields []spec.InspectField
	for i, dev := range op.devices {
		fields = append(fields, spec.InspectField{
			Label: fmt.Sprintf("dev%d", i),
			Value: formatDev(dev),
		})
	}
	return fields
}
