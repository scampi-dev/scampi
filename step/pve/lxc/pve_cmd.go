// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// pveCmd provides shared command helpers for all pve.lxc ops.
type pveCmd struct {
	id   int
	step spec.StepInstance
}

func (p pveCmd) runCmd(ctx context.Context, cmdr target.Command, opName, cmd string) error {
	result, err := cmdr.RunPrivileged(ctx, cmd)
	if err != nil {
		return p.cmdErr(opName, err.Error())
	}
	if result.ExitCode != 0 {
		return p.cmdErr(opName, result.Stderr)
	}
	return nil
}

func (p pveCmd) cmdErr(operation, stderr string) CommandFailedError {
	return CommandFailedError{
		Op:     operation,
		VMID:   p.id,
		Stderr: stderr,
		Source: p.step.Source,
	}
}

func (p pveCmd) inspectExists(ctx context.Context, cmdr target.Command) (exists bool, status string, err error) {
	result, err := cmdr.RunPrivileged(ctx, "pct list")
	if err != nil {
		return false, "", p.cmdErr("list", err.Error())
	}
	if result.ExitCode != 0 {
		return false, "", p.cmdErr("list", result.Stderr)
	}

	entries := parsePctList(result.Stdout)
	if _, ok := entries[p.id]; !ok {
		return false, "", nil
	}

	result, err = cmdr.RunPrivileged(ctx, fmt.Sprintf("pct status %d", p.id))
	if err != nil {
		return false, "", p.cmdErr("status", err.Error())
	}
	if result.ExitCode != 0 {
		return false, "", p.cmdErr("status", result.Stderr)
	}

	return true, parsePctStatus(result.Stdout), nil
}

func (p pveCmd) inspectConfig(ctx context.Context, cmdr target.Command) (pctConfig, error) {
	result, err := cmdr.RunPrivileged(ctx, fmt.Sprintf("pct config %d", p.id))
	if err != nil {
		return pctConfig{}, p.cmdErr("config", err.Error())
	}
	if result.ExitCode != 0 {
		return pctConfig{}, p.cmdErr("config", result.Stderr)
	}
	return parsePctConfig(result.Stdout), nil
}

// filterSetDrift returns only the drift details that map to `pct set` flags.
func filterSetDrift(drift []spec.DriftDetail) []spec.DriftDetail {
	var set []spec.DriftDetail
	for _, d := range drift {
		switch d.Field {
		case "cores", "cpulimit", "cpuunits", "memory", "swap",
			"hostname", "nameserver", "searchdomain",
			"tags", "description", "features", "onboot", "startup":
			set = append(set, d)
		}
	}
	return set
}

func hasNetworkDrift(drift []spec.DriftDetail) bool {
	for _, d := range drift {
		if strings.HasPrefix(d.Field, "network[") {
			return true
		}
	}
	return false
}

func parsedToLxcNet(p parsedNet) LxcNet {
	return LxcNet(p)
}

func hasDNSDrift(drift []spec.DriftDetail) bool {
	for _, d := range drift {
		if d.Field == "nameserver" || d.Field == "searchdomain" {
			return true
		}
	}
	return false
}

func hasDeviceDrift(drift []spec.DriftDetail) bool {
	for _, d := range drift {
		if strings.HasPrefix(d.Field, "device[") {
			return true
		}
	}
	return false
}

func parsedToLxcDevice(p parsedDev) LxcDevice {
	return LxcDevice(p)
}

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

// normalizeCPULimit strips trailing zeros for comparison.
// PVE stores "0.500000", user writes "0.5" — both mean the same thing.
func normalizeCPULimit(s string) string {
	if s == "" {
		return ""
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// dnsFingerprint returns "(none)" for empty values so the reboot
// check runner's empty-string guard doesn't swallow the comparison.
func dnsFingerprint(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func templateStr(t *LxcTemplate) string {
	if t == nil {
		return "(none)"
	}
	return t.templatePath()
}

func (p pveCmd) checkNode(ctx context.Context, cmdr target.Command, node string) error {
	result, err := cmdr.RunPrivileged(ctx, "hostname")
	if err != nil || result.ExitCode != 0 {
		return nil
	}
	actual := strings.TrimSpace(result.Stdout)
	if actual != node {
		return NodeMismatchError{
			Declared: node,
			Actual:   actual,
			Source:   p.step.Fields["node"].Value,
		}
	}
	return nil
}
