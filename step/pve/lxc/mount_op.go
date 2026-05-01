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

const mountLxcID = "step.pve.lxc.mount"

type mountLxcOp struct {
	sharedop.BaseOp
	pveCmd
	mounts []LxcMount
	step   spec.StepInstance
}

func (op *mountLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](mountLxcID, tgt)

	// Verify bind mount source directories exist on the host.
	for _, m := range op.mounts {
		if m.Kind != MountBind {
			continue
		}
		cmd := fmt.Sprintf("test -d %s", shellQuote(m.Source))
		r, err := cmdr.RunPrivileged(ctx, cmd)
		if err != nil {
			return spec.CheckUnsatisfied, nil, err
		}
		if r.ExitCode != 0 {
			return spec.CheckUnsatisfied, nil, BindSourceMissingError{
				Path:   m.Source,
				Source: op.step.Source,
			}
		}
	}

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}
	if !exists {
		if len(op.mounts) > 0 {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "mounts",
				Current: "(pending create)",
				Desired: fmt.Sprintf("%d mount(s)", len(op.mounts)),
			}}, nil
		}
		return spec.CheckSatisfied, nil, nil
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	drift := op.mountDrift(cfg)
	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
}

func (op *mountLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](mountLxcID, tgt)

	exists, _, err := op.inspectExists(ctx, cmdr)
	if err != nil || !exists {
		return spec.Result{}, err
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	drift := op.mountDrift(cfg)
	if len(drift) == 0 {
		return spec.Result{}, nil
	}

	if err := op.applyMountDrift(ctx, cmdr, cfg); err != nil {
		return spec.Result{}, err
	}
	return spec.Result{Changed: true}, nil
}

func (op *mountLxcOp) mountDrift(cfg pctConfig) []spec.DriftDetail {
	var drift []spec.DriftDetail
	maxMounts := max(len(cfg.Mounts), len(op.mounts))
	for i := range maxMounts {
		field := fmt.Sprintf("mount[%d]", i)
		if i >= len(op.mounts) {
			drift = append(drift, spec.DriftDetail{
				Field:   field,
				Current: formatMp(parsedToLxcMount(cfg.Mounts[i])),
				Desired: "(absent)",
			})
			continue
		}
		if i >= len(cfg.Mounts) {
			drift = append(drift, spec.DriftDetail{
				Field:   field,
				Current: "(absent)",
				Desired: formatMp(op.mounts[i]),
			})
			continue
		}
		desired := formatMp(op.mounts[i])
		current := formatMpForDrift(cfg.Mounts[i])
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

// formatMpForDrift formats a parsed mount for comparison against the
// desired formatMp output. For volume mounts, the parsed storage
// field contains "pool:volume-name" but formatMp produces "pool:size"
// — so we reconstruct the comparable form using storage + size.
func formatMpForDrift(p parsedMount) string {
	m := parsedToLxcMount(p)
	if m.Kind == MountVolume {
		// The parsed storage has "pool:volume-name" but for drift
		// comparison we only care about pool + size + mountpoint.
		return formatMp(m)
	}
	return formatMp(m)
}

func (op *mountLxcOp) applyMountDrift(
	ctx context.Context,
	cmdr target.Command,
	cfg pctConfig,
) error {
	maxMounts := max(len(cfg.Mounts), len(op.mounts))

	// Delete from high index to low to avoid index shifting.
	for i := maxMounts - 1; i >= 0; i-- {
		if i >= len(cfg.Mounts) {
			continue
		}
		if i < len(op.mounts) {
			desired := formatMp(op.mounts[i])
			current := formatMpForDrift(cfg.Mounts[i])
			if current == desired {
				continue
			}
		}
		cmd := fmt.Sprintf("pct set %d --delete mp%d", op.id, i)
		if err := op.runCmd(ctx, cmdr, "delete mount", cmd); err != nil {
			return err
		}
	}

	// Add/update from low to high.
	for i, m := range op.mounts {
		if i < len(cfg.Mounts) {
			desired := formatMp(m)
			current := formatMpForDrift(cfg.Mounts[i])
			if current == desired {
				continue
			}
		}
		cmd := fmt.Sprintf("pct set %d --mp%d %s", op.id, i, formatMp(m))
		if err := op.runCmd(ctx, cmdr, "set mount", cmd); err != nil {
			return err
		}
	}
	return nil
}

func (mountLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type mountLxcDesc struct {
	VMID   int
	Mounts int
}

func (d mountLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   mountLxcID,
		Text: `manage mounts for LXC {{.VMID}} ({{.Mounts}} mount(s))`,
		Data: d,
	}
}

func (op *mountLxcOp) OpDescription() spec.OpDescription {
	return mountLxcDesc{VMID: op.id, Mounts: len(op.mounts)}
}

func (op *mountLxcOp) Inspect() []spec.InspectField {
	var fields []spec.InspectField
	for i, m := range op.mounts {
		fields = append(fields, spec.InspectField{
			Label: fmt.Sprintf("mp%d", i),
			Value: formatMp(m),
		})
	}
	return fields
}
