// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"fmt"
	"strconv"
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

// State
// -----------------------------------------------------------------------------

type State uint8

const (
	StateRunning State = iota + 1
	StateStopped
	StateAbsent
)

const (
	stateRunning = "running"
	stateStopped = "stopped"
	stateAbsent  = "absent"
)

var StateValues = []string{stateRunning, stateStopped, stateAbsent}

func (s State) String() string {
	switch s {
	case StateRunning:
		return stateRunning
	case StateStopped:
		return stateStopped
	case StateAbsent:
		return stateAbsent
	default:
		return "unknown"
	}
}

func parseState(s string) State {
	switch s {
	case stateRunning:
		return StateRunning
	case stateStopped:
		return StateStopped
	case stateAbsent:
		return StateAbsent
	default:
		panic(errs.BUG("invalid lxc state %q — should have been caught by validate", s))
	}
}

// Config
// -----------------------------------------------------------------------------

type (
	LXC       struct{}
	LxcConfig struct {
		_ struct{} `summary:"Manage LXC container lifecycle on Proxmox VE via pct"`

		ID       int         `step:"Container VMID (unique per cluster)" example:"100"`
		Node     string      `step:"PVE node name" example:"pve1"`
		Template LxcTemplate `step:"OS template"`
		Hostname string      `step:"Container hostname" example:"pihole"`
		State    string      `step:"Desired state" default:"running"`
		Cores    int         `step:"CPU cores" default:"1"`
		Memory   string      `step:"Memory with unit (e.g. 512M, 2G)" default:"512M"`
		Storage  string      `step:"Storage pool for rootfs" default:"local-zfs"`
		Size     string      `step:"Root disk size with unit (e.g. 8G, 500M)" default:"8G"`
		Network  LxcNet      `step:"Network configuration"`
		Desc     string      `step:"Human-readable description" optional:"true"`
	}
	LxcTemplate struct {
		Storage string `step:"Storage pool holding the template" default:"local"`
		Name    string `step:"Template filename" example:"debian-12-standard_12.7-1_amd64.tar.zst"`
	}
	LxcNet struct {
		Bridge string `step:"Bridge interface" default:"vmbr0"`
		IP     string `step:"IP address in CIDR or dhcp" example:"10.10.10.10/24"`
		Gw     string `step:"Gateway" optional:"true" example:"10.10.10.1"`
	}
)

func (*LxcConfig) FieldEnumValues() map[string][]string {
	return map[string][]string{
		"state": StateValues,
	}
}

func (LXC) Kind() string   { return "pve.lxc" }
func (LXC) NewConfig() any { return &LxcConfig{} }

func (LXC) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*LxcConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &LxcConfig{}, step.Config)
	}

	warn := cfg.validate(step)
	if warn != nil {
		// Fatal errors → no action. Warnings → action + warning.
		if _, ok := warn.(SizeTruncatedWarning); !ok {
			return nil, warn
		}
	}

	act := &lxcAction{
		desc:      cfg.Desc,
		id:        cfg.ID,
		node:      cfg.Node,
		template:  cfg.Template,
		hostname:  cfg.Hostname,
		state:     parseState(cfg.State),
		cores:     cfg.Cores,
		memoryMiB: sizeToMiB(cfg.Memory),
		storage:   cfg.Storage,
		sizeGiB:   sizeToGiB(cfg.Size),
		network:   cfg.Network,
		step:      step,
	}
	return act, warn // warn is nil or SizeTruncatedWarning (ImpactNone)
}

// templatePath returns the full PVE template path for pct create.
func (t LxcTemplate) templatePath() string {
	return t.Storage + ":vztmpl/" + t.Name
}

// parseSizeSpec parses a human-readable size like "512M", "2G", "1T" into MiB.
// Accepted units: M (MiB), G (GiB), T (TiB). Returns an error for invalid formats.
//
// bare-error: internal validation helper — callers wrap the result in typed diagnostic errors
func parseSizeSpec(s string) (int, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		// bare-error: wrapped by InvalidConfigError in validate()
		return 0, "", errs.Errorf("empty size")
	}

	unit := strings.ToUpper(s[len(s)-1:])
	numStr := s[:len(s)-1]

	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil || f <= 0 {
		// bare-error: wrapped by InvalidConfigError in validate()
		return 0, "", errs.Errorf("invalid size %q", s)
	}

	switch unit {
	case "M":
		return int(f), "M", nil
	case "G":
		return int(f * 1024), "G", nil
	case "T":
		return int(f * 1024 * 1024), "T", nil
	default:
		// bare-error: wrapped by InvalidConfigError in validate()
		return 0, "", errs.Errorf("invalid size %q — use M, G, or T suffix (e.g. 512M, 8G, 1T)", s)
	}
}

// sizeToMiB parses a size string to MiB. Panics on invalid input (validation should catch first).
func sizeToMiB(s string) int {
	n, _, err := parseSizeSpec(s)
	if err != nil {
		panic(errs.BUG("invalid size %q in sizeToMiB — should have been caught by validate", s))
	}
	return n
}

// sizeToGiB parses a disk size string to GiB. Returns 0 if < 1G.
func sizeToGiB(s string) int {
	mib, _, err := parseSizeSpec(s)
	if err != nil {
		return 0
	}
	return mib / 1024
}

func (c *LxcConfig) validate(step spec.StepInstance) error {
	if c.ID < 100 {
		return InvalidConfigError{
			Field:  "id",
			Reason: "VMID must be >= 100 (PVE reserves 0-99)",
			Source: step.Fields["id"].Value,
		}
	}
	if c.Node == "" {
		return InvalidConfigError{
			Field:  "node",
			Reason: "node name is required",
			Source: step.Fields["node"].Value,
		}
	}
	if c.Hostname == "" {
		return InvalidConfigError{
			Field:  "hostname",
			Reason: "hostname is required",
			Source: step.Fields["hostname"].Value,
		}
	}
	if c.Template.Name == "" {
		return InvalidConfigError{
			Field:  "template.name",
			Reason: "template name is required",
			Source: step.Fields["template"].Value,
		}
	}
	if _, _, err := parseSizeSpec(c.Memory); err != nil {
		return InvalidConfigError{
			Field:  "memory",
			Reason: fmt.Sprintf("%s — use M or G suffix (e.g. 512M, 2G)", err),
			Source: step.Fields["memory"].Value,
		}
	}
	sizeMiB, sizeUnit, err := parseSizeSpec(c.Size)
	if err != nil {
		return InvalidConfigError{
			Field:  "size",
			Reason: fmt.Sprintf("%s — use M, G, or T suffix (e.g. 8G, 500M, 1T)", err),
			Source: step.Fields["size"].Value,
		}
	}
	sizeGiB := sizeMiB / 1024
	if sizeGiB < 1 {
		return InvalidConfigError{
			Field:  "size",
			Reason: fmt.Sprintf("%s is less than the minimum 1G — PVE rootfs must be at least 1G", c.Size),
			Source: step.Fields["size"].Value,
		}
	}
	if c.Network.IP == "" {
		return InvalidConfigError{
			Field:  "network.ip",
			Reason: "network IP address is required",
			Source: step.Fields["network"].Value,
		}
	}
	if sizeUnit != "M" && sizeMiB%1024 != 0 {
		return SizeTruncatedWarning{
			Input:   c.Size,
			Rounded: fmt.Sprintf("%dG", sizeGiB),
			Exact:   fmt.Sprintf("%dM", sizeMiB),
			Source:  step.Fields["size"].Value,
		}
	}
	return nil
}

// Action
// -----------------------------------------------------------------------------

type lxcAction struct {
	desc      string
	id        int
	node      string
	template  LxcTemplate
	hostname  string
	state     State
	cores     int
	memoryMiB int
	storage   string
	sizeGiB   int
	network   LxcNet
	step      spec.StepInstance
}

func (a *lxcAction) Desc() string { return a.desc }
func (a *lxcAction) Kind() string { return "pve.lxc" }

func (a *lxcAction) Ops() []spec.Op {
	dlOp := &downloadTemplateOp{
		template: a.template,
		step:     a.step,
	}
	dlOp.SetAction(a)

	lxcOp := &ensureLxcOp{
		id:        a.id,
		node:      a.node,
		template:  a.template,
		hostname:  a.hostname,
		state:     a.state,
		cores:     a.cores,
		memoryMiB: a.memoryMiB,
		storage:   a.storage,
		sizeGiB:   a.sizeGiB,
		network:   a.network,
		step:      a.step,
	}
	lxcOp.SetAction(a)
	lxcOp.AddDependency(dlOp)

	return []spec.Op{dlOp, lxcOp}
}
