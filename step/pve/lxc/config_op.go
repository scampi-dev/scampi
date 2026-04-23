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
	cpu        LxcCPU
	memoryMiB  int
	swapMiB    int
	storage    string
	privileged bool
	dns        LxcDNS
	features   *LxcFeatures
	startup    *LxcStartup
	networks   []LxcNet
	devices    []LxcDevice
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

	if hasNetworkDrift(drift) {
		if err := op.applyNetworkDrift(ctx, cmdr, cfg); err != nil {
			return spec.Result{}, err
		}
		changed = true
	}

	if hasDeviceDrift(drift) {
		if err := op.applyDeviceDrift(ctx, cmdr, cfg); err != nil {
			return spec.Result{}, err
		}
		changed = true
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

	// Network drift — compare per-index.
	maxNets := max(len(cfg.Nets), len(op.networks))
	for i := range maxNets {
		field := fmt.Sprintf("network[%d]", i)
		if i >= len(op.networks) {
			// Extra NIC on host — needs removal.
			drift = append(drift, spec.DriftDetail{
				Field:   field,
				Current: formatNet(i, parsedToLxcNet(cfg.Nets[i])),
				Desired: "(absent)",
			})
			continue
		}
		if i >= len(cfg.Nets) {
			// Missing NIC on host — needs creation.
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

	// Device drift — compare per-index.
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

func (op *configLxcOp) applyNetworkDrift(
	ctx context.Context,
	cmdr target.Command,
	cfg pctConfig,
) error {
	maxNets := max(len(cfg.Nets), len(op.networks))

	// Phase 1: delete changed/removed NICs in reverse order.
	// Deleting first avoids veth conflicts when hotplugging reordered interfaces.
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

	// Phase 2: recreate desired NICs.
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

func (op *configLxcOp) applyDeviceDrift(
	ctx context.Context,
	cmdr target.Command,
	cfg pctConfig,
) error {
	maxDevs := max(len(cfg.Devs), len(op.devices))

	// Phase 1: delete changed/removed devices in reverse order.
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

	// Phase 2: recreate desired devices.
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
	fields := []spec.InspectField{
		{Label: "vmid", Value: fmt.Sprintf("%d", op.id)},
		{Label: "hostname", Value: op.hostname},
		{Label: "cores", Value: fmt.Sprintf("%d", op.cpu.Cores)},
		{Label: "memory", Value: fmt.Sprintf("%d MiB", op.memoryMiB)},
		{Label: "swap", Value: fmt.Sprintf("%d MiB", op.swapMiB)},
	}
	for i, net := range op.networks {
		fields = append(fields, spec.InspectField{
			Label: fmt.Sprintf("net%d", i),
			Value: formatNet(i, net),
		})
	}
	for i, dev := range op.devices {
		fields = append(fields, spec.InspectField{
			Label: fmt.Sprintf("dev%d", i),
			Value: formatDev(dev),
		})
	}
	fields = append(fields, spec.InspectField{Label: "tags", Value: strings.Join(op.tags, ", ")})
	return fields
}
