// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const rebootLxcID = "step.pve.lxc.reboot"

// rebootCheck probes the running container for a field that needs a reboot
// to take effect. Returns nil if the field is already converged.
type rebootCheck struct {
	field   string
	desired string
	probe   func(ctx context.Context, cmdr target.Command, id int) string
}

// rebootChecks lists all fields that are written to config immediately
// but only take effect after a container reboot.
func buildRebootChecks(op *rebootLxcOp) []rebootCheck {
	checks := []rebootCheck{
		{
			field:   "hostname",
			desired: op.hostname,
			probe: func(ctx context.Context, cmdr target.Command, id int) string {
				r, err := cmdr.RunPrivileged(ctx, fmt.Sprintf("pct exec %d -- hostname", id))
				if err != nil || r.ExitCode != 0 {
					return ""
				}
				return strings.TrimSpace(r.Stdout)
			},
		},
	}

	// Features: compare running config's features against desired.
	// pct config shows the written value; the running container may
	// still have the old one. We compare desired vs current config
	// and if they differ, the container needs a reboot.
	if op.features != nil {
		checks = append(checks, rebootCheck{
			field:   "features",
			desired: formatFeatures(op.features),
			probe: func(ctx context.Context, cmdr target.Command, _ int) string {
				cfg, err := op.inspectConfig(ctx, cmdr)
				if err != nil {
					return ""
				}
				return formatFeatures(&cfg.Features)
			},
		})
	}

	// DNS: resolv.conf is written at container start. Compare
	// config state (pre-cfgOp) against desired. Catches both
	// additions and removals at Check time.
	checks = append(checks, rebootCheck{
		field:   "nameserver",
		desired: dnsFingerprint(op.dns.Nameserver),
		probe: func(ctx context.Context, cmdr target.Command, _ int) string {
			cfg, err := op.inspectConfig(ctx, cmdr)
			if err != nil {
				return ""
			}
			return dnsFingerprint(cfg.Nameserver)
		},
	})
	checks = append(checks, rebootCheck{
		field:   "searchdomain",
		desired: dnsFingerprint(op.dns.Searchdomain),
		probe: func(ctx context.Context, cmdr target.Command, _ int) string {
			cfg, err := op.inspectConfig(ctx, cmdr)
			if err != nil {
				return ""
			}
			return dnsFingerprint(cfg.Searchdomain)
		},
	})

	// Devices: devN entries are read at container start. Compare
	// what the config says (pre-cfgOp state during Check) against
	// what's actually inside the running container. Catches:
	//  - additions: config has no dev, desired does → cfgOp will add,
	//    reboot needed (config ≠ desired at Check time)
	//  - removals: config has dev, desired doesn't → cfgOp will delete,
	//    reboot needed (config ≠ desired at Check time)
	//  - interrupted runs: config already matches desired but the
	//    container wasn't rebooted → probe running state to verify
	{
		checks = append(checks, rebootCheck{
			field:   "devices",
			desired: devicesFingerprint(op.devices),
			probe: func(ctx context.Context, cmdr target.Command, id int) string {
				// First compare config vs desired (catches pending
				// cfgOp changes before they're applied).
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
				// Config matches desired — verify the running
				// container actually reflects it (catches interrupted
				// runs where config was updated but reboot didn't fire).
				for _, d := range op.devices {
					cmd := fmt.Sprintf("pct exec %d -- test -e %s", id, shellQuote(d.Path))
					r, err := cmdr.RunPrivileged(ctx, cmd)
					if err != nil || r.ExitCode != 0 {
						return "stale (device missing)"
					}
				}
				// If desired is empty and config is empty, check
				// common device paths that might be stale.
				if len(op.devices) == 0 && len(cfg.Devs) == 0 {
					// We can't enumerate — but /dev/dri is the most
					// common passthrough path. Check if it exists
					// when it shouldn't.
					cmd := fmt.Sprintf("pct exec %d -- test -d /dev/dri", id)
					r, err := cmdr.RunPrivileged(ctx, cmd)
					if err == nil && r.ExitCode == 0 {
						return "stale (device remnant)"
					}
				}
				return desiredFP
			},
		})
	}

	return checks
}

type rebootLxcOp struct {
	sharedops.BaseOp
	pveCmd
	hostname string
	features *LxcFeatures
	dns      LxcDNS
	devices  []LxcDevice
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
	for _, check := range buildRebootChecks(op) {
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

	// Re-check: container may have been stopped since Check ran.
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
