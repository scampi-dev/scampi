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

const ensureLxcID = "step.pve.lxc"

type ensureLxcOp struct {
	sharedops.BaseOp
	id            int
	node          string
	template      *LxcTemplate
	hostname      string
	state         State
	cores         int
	memoryMiB     int
	swapMiB       int
	storage       string
	sizeGiB       int
	privileged    bool
	network       LxcNet
	tags          []string
	sshPublicKeys []string
	step          spec.StepInstance
}

func (op *ensureLxcOp) Check(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cmdr := target.Must[target.Command](ensureLxcID, tgt)

	if err := op.checkNode(ctx, cmdr); err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	exists, status, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	// Absent: satisfied if container doesn't exist.
	if op.state == StateAbsent {
		if !exists {
			return spec.CheckSatisfied, nil, nil
		}
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "state",
			Current: "present",
			Desired: stateAbsent,
		}}, nil
	}

	if !exists {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "state",
			Current: "(absent)",
			Desired: op.state.String(),
		}}, nil
	}

	// Container exists — fetch config and detect drift.
	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	// Immutable fields: reject at check time.
	if err := op.checkImmutables(cfg); err != nil {
		return spec.CheckUnsatisfied, nil, err
	}

	// Mutable drift.
	drift := op.configDrift(cfg)

	// State drift.
	switch op.state {
	case StateRunning:
		if status != stateRunning {
			drift = append(drift, spec.DriftDetail{
				Field:   "state",
				Current: status,
				Desired: stateRunning,
			})
		}
	case StateStopped:
		if status != stateStopped {
			drift = append(drift, spec.DriftDetail{
				Field:   "state",
				Current: status,
				Desired: stateStopped,
			})
		}
	}

	if len(drift) > 0 {
		return spec.CheckUnsatisfied, drift, nil
	}
	return spec.CheckSatisfied, nil, nil
}

func (op *ensureLxcOp) Execute(
	ctx context.Context,
	_ source.Source,
	tgt target.Target,
) (spec.Result, error) {
	cmdr := target.Must[target.Command](ensureLxcID, tgt)

	exists, status, err := op.inspectExists(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	// Absent: shutdown if running, then destroy.
	if op.state == StateAbsent {
		if !exists {
			return spec.Result{}, nil
		}
		return op.executeDestroy(ctx, cmdr, status)
	}

	if !exists {
		return op.executeCreate(ctx, cmdr, tgt)
	}

	cfg, err := op.inspectConfig(ctx, cmdr)
	if err != nil {
		return spec.Result{}, err
	}

	changed := false

	// Apply mutable config drift via pct set.
	drift := op.configDrift(cfg)
	setDrift := filterSetDrift(drift)
	if len(setDrift) > 0 {
		if err := op.runCmd(ctx, cmdr, "set", buildSetCmd(op.id, setDrift)); err != nil {
			return spec.Result{}, err
		}
		changed = true
	}

	// Apply network drift via pct set --net0.
	if hasNetworkDrift(drift) {
		cmd := fmt.Sprintf("pct set %d --net0 %s", op.id, formatNet0(op.network))
		if err := op.runCmd(ctx, cmdr, "set network", cmd); err != nil {
			return spec.Result{}, err
		}
		changed = true
	}

	// Resize rootfs if needed (grow only).
	if sizeDrift := findDrift(drift, "size"); sizeDrift != nil {
		cmd := fmt.Sprintf("pct resize %d rootfs %dG", op.id, op.sizeGiB)
		if err := op.runCmd(ctx, cmdr, "resize", cmd); err != nil {
			return spec.Result{}, err
		}
		changed = true
	}

	// State transitions.
	switch op.state {
	case StateRunning:
		if status != stateRunning {
			if err := op.runCmd(ctx, cmdr, "start", fmt.Sprintf("pct start %d", op.id)); err != nil {
				return spec.Result{}, err
			}
			changed = true
		}
	case StateStopped:
		if status != stateStopped {
			if err := op.runCmd(ctx, cmdr, "shutdown", fmt.Sprintf("pct shutdown %d --timeout 30", op.id)); err != nil {
				return spec.Result{}, err
			}
			changed = true
		}
	}

	return spec.Result{Changed: changed}, nil
}

// Inspect helpers
// -----------------------------------------------------------------------------

func (op *ensureLxcOp) inspectExists(ctx context.Context, cmdr target.Command) (exists bool, status string, err error) {
	result, err := cmdr.RunPrivileged(ctx, "pct list")
	if err != nil {
		return false, "", op.cmdErrWrap("list", err)
	}
	if result.ExitCode != 0 {
		return false, "", op.cmdErrStr("list", result.Stderr)
	}

	entries := parsePctList(result.Stdout)
	if _, ok := entries[op.id]; !ok {
		return false, "", nil
	}

	result, err = cmdr.RunPrivileged(ctx, fmt.Sprintf("pct status %d", op.id))
	if err != nil {
		return false, "", op.cmdErrWrap("status", err)
	}
	if result.ExitCode != 0 {
		return false, "", op.cmdErrStr("status", result.Stderr)
	}

	return true, parsePctStatus(result.Stdout), nil
}

func (op *ensureLxcOp) inspectConfig(ctx context.Context, cmdr target.Command) (pctConfig, error) {
	result, err := cmdr.RunPrivileged(ctx, fmt.Sprintf("pct config %d", op.id))
	if err != nil {
		return pctConfig{}, op.cmdErrWrap("config", err)
	}
	if result.ExitCode != 0 {
		return pctConfig{}, op.cmdErrStr("config", result.Stderr)
	}
	return parsePctConfig(result.Stdout), nil
}

// Drift detection
// -----------------------------------------------------------------------------

func (op *ensureLxcOp) checkNode(ctx context.Context, cmdr target.Command) error {
	result, err := cmdr.RunPrivileged(ctx, "hostname")
	if err != nil {
		return nil // can't verify, skip
	}
	if result.ExitCode != 0 {
		return nil
	}
	actual := strings.TrimSpace(result.Stdout)
	if actual != op.node {
		return NodeMismatchError{
			Declared: op.node,
			Actual:   actual,
			Source:   op.step.Fields["node"].Value,
		}
	}
	return nil
}

func (op *ensureLxcOp) checkImmutables(cfg pctConfig) error {
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

	// Size can grow but not shrink.
	if cfg.Size != "" {
		currentGiB := parseSizeGiB(cfg.Size)
		if op.sizeGiB > 0 && currentGiB > 0 && op.sizeGiB < currentGiB {
			return ResizeShrinkError{
				Current: fmt.Sprintf("%dG", currentGiB),
				Desired: fmt.Sprintf("%dG", op.sizeGiB),
				Source:  op.step.Fields["size"].Value,
			}
		}
	}

	return nil
}

func (op *ensureLxcOp) configDrift(cfg pctConfig) []spec.DriftDetail {
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

	// Network drift (any sub-field).
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

	// Size drift (grow only — shrink rejected in checkImmutables).
	if cfg.Size != "" {
		currentGiB := parseSizeGiB(cfg.Size)
		if op.sizeGiB > 0 && currentGiB > 0 && op.sizeGiB > currentGiB {
			drift = append(drift, spec.DriftDetail{
				Field:   "size",
				Current: fmt.Sprintf("%dG", currentGiB),
				Desired: fmt.Sprintf("%dG", op.sizeGiB),
			})
		}
	}

	return drift
}

// Drift helpers
// -----------------------------------------------------------------------------

// filterSetDrift returns only the drift details that map to `pct set` flags
// (cores, memory, hostname). Network and size are handled separately.
func filterSetDrift(drift []spec.DriftDetail) []spec.DriftDetail {
	var set []spec.DriftDetail
	for _, d := range drift {
		switch d.Field {
		case "cores", "memory", "swap", "hostname", "tags", "description":
			set = append(set, d)
		}
	}
	return set
}

func hasNetworkDrift(drift []spec.DriftDetail) bool {
	for _, d := range drift {
		if strings.HasPrefix(d.Field, "network.") {
			return true
		}
	}
	return false
}

func findDrift(drift []spec.DriftDetail, field string) *spec.DriftDetail {
	for i := range drift {
		if drift[i].Field == field {
			return &drift[i]
		}
	}
	return nil
}

// parseSizeGiB extracts the numeric GiB value from "4G", "10G", "4", etc.
func parseSizeGiB(s string) int {
	s = strings.TrimRight(s, "GgTt")
	n, _ := strconv.Atoi(s)
	return n
}

func valueOrNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func templateStr(t *LxcTemplate) string {
	if t == nil {
		return "(none)"
	}
	return t.templatePath()
}

// Execute helpers
// -----------------------------------------------------------------------------

func (op *ensureLxcOp) executeDestroy(ctx context.Context, cmdr target.Command, status string) (spec.Result, error) {
	if status == stateRunning {
		if err := op.runCmd(ctx, cmdr, "shutdown", fmt.Sprintf("pct shutdown %d --timeout 30", op.id)); err != nil {
			return spec.Result{}, err
		}
	}
	if err := op.runCmd(ctx, cmdr, "destroy", fmt.Sprintf("pct destroy %d --purge", op.id)); err != nil {
		return spec.Result{}, err
	}
	return spec.Result{Changed: true}, nil
}

func (op *ensureLxcOp) executeCreate(ctx context.Context, cmdr target.Command, tgt target.Target) (spec.Result, error) {
	cfg := *op.Action().(*lxcAction)
	createCmd := buildCreateCmd(cfg)

	// SSH keys: write to temp file, pass to pct create, clean up after.
	var keyFile string
	if len(op.sshPublicKeys) > 0 {
		keyFile = fmt.Sprintf("/tmp/.scampi-ssh-keys-%d", op.id)
		fs := target.Must[target.Filesystem](ensureLxcID, tgt)
		keyContent := strings.Join(op.sshPublicKeys, "\n") + "\n"
		if err := fs.WriteFile(ctx, keyFile, []byte(keyContent)); err != nil {
			return spec.Result{}, op.cmdErrWrap("write ssh keys", err)
		}
		createCmd += " --ssh-public-keys " + shellQuote(keyFile)
	}

	err := op.runCmd(ctx, cmdr, "create", createCmd)

	// Clean up key file regardless of create outcome.
	if keyFile != "" {
		fs := target.Must[target.Filesystem](ensureLxcID, tgt)
		_ = fs.Remove(ctx, keyFile)
	}

	if err != nil {
		return spec.Result{}, err
	}

	if op.state == StateRunning {
		if err := op.runCmd(ctx, cmdr, "start", fmt.Sprintf("pct start %d", op.id)); err != nil {
			return spec.Result{}, err
		}
	}

	return spec.Result{Changed: true}, nil
}

// Command helpers
// -----------------------------------------------------------------------------

func (op *ensureLxcOp) runCmd(ctx context.Context, cmdr target.Command, opName, cmd string) error {
	result, err := cmdr.RunPrivileged(ctx, cmd)
	if err != nil {
		return op.cmdErrWrap(opName, err)
	}
	if result.ExitCode != 0 {
		return op.cmdErrStr(opName, result.Stderr)
	}
	return nil
}

func (op *ensureLxcOp) cmdErrWrap(operation string, err error) CommandFailedError {
	return CommandFailedError{
		Op:     operation,
		VMID:   op.id,
		Stderr: err.Error(),
		Source: op.step.Source,
	}
}

func (op *ensureLxcOp) cmdErrStr(operation, stderr string) CommandFailedError {
	return CommandFailedError{
		Op:     operation,
		VMID:   op.id,
		Stderr: stderr,
		Source: op.step.Source,
	}
}

func (ensureLxcOp) RequiredCapabilities() capability.Capability {
	return capability.PVE | capability.Command
}

// OpDescription
// -----------------------------------------------------------------------------

type ensureLxcDesc struct {
	VMID     int
	Hostname string
	State    string
}

func (d ensureLxcDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureLxcID,
		Text: `ensure LXC {{.VMID}} "{{.Hostname}}" is {{.State}}`,
		Data: d,
	}
}

func (op *ensureLxcOp) OpDescription() spec.OpDescription {
	return ensureLxcDesc{
		VMID:     op.id,
		Hostname: op.hostname,
		State:    op.state.String(),
	}
}

func (op *ensureLxcOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "vmid", Value: fmt.Sprintf("%d", op.id)},
		{Label: "node", Value: op.node},
		{Label: "hostname", Value: op.hostname},
		{Label: "state", Value: op.state.String()},
		{Label: "template", Value: templateStr(op.template)},
		{Label: "cores", Value: fmt.Sprintf("%d", op.cores)},
		{Label: "memory", Value: fmt.Sprintf("%d MiB", op.memoryMiB)},
		{Label: "swap", Value: fmt.Sprintf("%d MiB", op.swapMiB)},
		{Label: "storage", Value: op.storage},
		{Label: "size", Value: fmt.Sprintf("%dG", op.sizeGiB)},
		{Label: "network", Value: formatNet0(op.network)},
		{Label: "tags", Value: strings.Join(op.tags, ", ")},
	}
}
