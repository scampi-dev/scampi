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
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/target"
)

const configLxcID = "step.pve.lxc.config"

type configLxcOp struct {
	sharedop.BaseOp
	pveCmd
	node       string
	hostname   string
	cpu        LxcCPU
	memoryMiB  int
	swapMiB    int
	storage    string
	privileged bool
	dns        LxcDNS
	features   *LxcFeatures
	startup    *LxcStartup
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

	exists, status, err := op.inspectExists(ctx, cmdr)
	if err != nil || !exists {
		return spec.Result{}, err
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	changed := false
	needsReboot := false

	drift := op.configDrift(cfg)
	setDrift := filterSetDrift(drift)
	if len(setDrift) > 0 {
		if err := op.runCmd(ctx, cmdr, "set", buildSetCmd(op.id, setDrift)); err != nil {
			return spec.Result{}, err
		}
		changed = true
		if hasDNSDrift(drift) {
			needsReboot = true
		}
	}

	// DNS changes take effect on container restart (PVE writes
	// resolv.conf at start). Reboot inline because the reboot op
	// can't reliably detect stale DNS when desired is empty.
	if needsReboot && status == stateRunning {
		cmd := fmt.Sprintf("pct reboot %d --timeout 30", op.id)
		if err := op.runCmd(ctx, cmdr, "reboot", cmd); err != nil {
			return spec.Result{}, err
		}
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

	if cfg.Cores != 0 && cfg.Cores != op.cpu.Cores {
		drift = append(drift, spec.DriftDetail{
			Field:   "cores",
			Current: strconv.Itoa(cfg.Cores),
			Desired: strconv.Itoa(op.cpu.Cores),
		})
	}
	if normalizeCPULimit(cfg.CPULimit) != normalizeCPULimit(op.cpu.Limit) {
		drift = append(drift, spec.DriftDetail{
			Field:   "cpulimit",
			Current: valueOrNone(cfg.CPULimit),
			Desired: valueOrNone(op.cpu.Limit),
		})
	}
	if cfg.CPUUnits != op.cpu.Weight {
		drift = append(drift, spec.DriftDetail{
			Field:   "cpuunits",
			Current: strconv.Itoa(cfg.CPUUnits),
			Desired: strconv.Itoa(op.cpu.Weight),
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
	if cfg.Nameserver != op.dns.Nameserver {
		drift = append(drift, spec.DriftDetail{
			Field:   "nameserver",
			Current: valueOrNone(cfg.Nameserver),
			Desired: valueOrNone(op.dns.Nameserver),
		})
	}
	if cfg.Searchdomain != op.dns.Searchdomain {
		drift = append(drift, spec.DriftDetail{
			Field:   "searchdomain",
			Current: valueOrNone(cfg.Searchdomain),
			Desired: valueOrNone(op.dns.Searchdomain),
		})
	}

	// Startup drift.
	desiredOnBoot := 0
	desiredStartup := ""
	if op.startup != nil {
		if op.startup.OnBoot {
			desiredOnBoot = 1
		}
		desiredStartup = formatStartup(op.startup)
	}
	if cfg.OnBoot != desiredOnBoot {
		drift = append(drift, spec.DriftDetail{
			Field:   "onboot",
			Current: strconv.Itoa(cfg.OnBoot),
			Desired: strconv.Itoa(desiredOnBoot),
		})
	}
	currentStartup := formatStartup(&cfg.Startup)
	if currentStartup != desiredStartup {
		drift = append(drift, spec.DriftDetail{
			Field:   "startup",
			Current: valueOrNone(currentStartup),
			Desired: valueOrNone(desiredStartup),
		})
	}

	desiredFeat := formatFeatures(op.features)
	currentFeat := formatFeatures(&cfg.Features)
	if currentFeat != desiredFeat {
		drift = append(drift, spec.DriftDetail{
			Field:   "features",
			Current: valueOrNone(currentFeat),
			Desired: valueOrNone(desiredFeat),
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

	return drift
}

func (op *configLxcOp) RebootChecks() []rebootCheck {
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
	return checks
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
		{Label: "cores", Value: fmt.Sprintf("%d", op.cpu.Cores)},
		{Label: "memory", Value: fmt.Sprintf("%d MiB", op.memoryMiB)},
		{Label: "swap", Value: fmt.Sprintf("%d MiB", op.swapMiB)},
		{Label: "tags", Value: strings.Join(op.tags, ", ")},
	}
}
