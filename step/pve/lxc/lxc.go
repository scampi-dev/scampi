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

		ID            int          `step:"Container VMID (unique per cluster)" example:"100"`
		Node          string       `step:"PVE node name" example:"pve1"`
		Template      *LxcTemplate `step:"OS template" optional:"true"`
		Hostname      string       `step:"Container hostname" example:"pihole"`
		State         string       `step:"Desired state" default:"running"`
		Cores         int          `step:"CPU cores" default:"1"`
		Memory        string       `step:"Memory with unit (e.g. 512M, 2G)" default:"512M"`
		Swap          string       `step:"Swap with unit (e.g. 512M, 2G) — defaults to memory value" optional:"true"`
		Storage       string       `step:"Storage pool for rootfs" default:"local-zfs"`
		Size          string       `step:"Root disk size with unit (e.g. 8G, 500M)" default:"8G"`
		Privileged    bool         `step:"Run as privileged container (less secure)" default:"false"`
		Features      *LxcFeatures `step:"Advanced LXC features" optional:"true"`
		Startup       *LxcStartup  `step:"Startup/shutdown ordering" optional:"true"`
		Network       LxcNet       `step:"Network configuration"`
		Tags          []string     `step:"PVE tags" optional:"true"`
		SSHPublicKeys []string     `step:"SSH public keys for root" optional:"true"`
		Desc          string       `step:"Human-readable description" optional:"true"`
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
	LxcStartup struct {
		OnBoot bool `step:"Start on host boot" default:"false"`
		Order  int  `step:"Startup sequence number" default:"0"`
		Up     int  `step:"Seconds delay before next container starts" default:"0"`
		Down   int  `step:"Seconds delay before next container stops" default:"0"`
	}
	LxcFeatures struct {
		Nesting    bool     `step:"Allow nesting (required by systemd)" default:"false"`
		Keyctl     bool     `step:"Allow keyctl syscall (required for Docker)" default:"false"`
		Fuse       bool     `step:"Allow FUSE filesystems" default:"false"`
		Mknod      bool     `step:"Allow mknod for device nodes" default:"false"`
		ForceRwSys bool     `step:"Mount /sys as rw in unprivileged containers" default:"false"`
		Mount      []string `step:"Allowed mount filesystem types" optional:"true"`
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
		desc:          cfg.Desc,
		id:            cfg.ID,
		node:          cfg.Node,
		template:      cfg.Template,
		hostname:      cfg.Hostname,
		state:         parseState(cfg.State),
		cores:         cfg.Cores,
		memoryMiB:     sizeToMiB(cfg.Memory),
		swapMiB:       resolveSwap(cfg.Swap, cfg.Memory),
		storage:       cfg.Storage,
		sizeGiB:       sizeToGiB(cfg.Size),
		privileged:    cfg.Privileged,
		features:      cfg.Features,
		startup:       cfg.Startup,
		network:       cfg.Network,
		tags:          cfg.Tags,
		sshPublicKeys: cfg.SSHPublicKeys,
		step:          step,
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

// resolveSwap returns swap MiB — uses the explicit swap value if set, otherwise
// clamps to the memory value.
func resolveSwap(swap, memory string) int {
	if swap != "" {
		return sizeToMiB(swap)
	}
	return sizeToMiB(memory)
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
	if c.State != stateAbsent && (c.Template == nil || c.Template.Name == "") {
		return InvalidConfigError{
			Field:  "template",
			Reason: "template is required when state is not absent",
			Source: step.Source,
		}
	}
	if _, _, err := parseSizeSpec(c.Memory); err != nil {
		return InvalidConfigError{
			Field:  "memory",
			Reason: fmt.Sprintf("%s — use M or G suffix (e.g. 512M, 2G)", err),
			Source: step.Fields["memory"].Value,
		}
	}
	if c.Swap != "" {
		if _, _, err := parseSizeSpec(c.Swap); err != nil {
			return InvalidConfigError{
				Field:  "swap",
				Reason: fmt.Sprintf("%s — use M or G suffix (e.g. 512M, 2G)", err),
				Source: step.Fields["swap"].Value,
			}
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
	desc          string
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
	features      *LxcFeatures
	startup       *LxcStartup
	network       LxcNet
	tags          []string
	sshPublicKeys []string
	step          spec.StepInstance
}

func (a *lxcAction) Desc() string { return a.desc }
func (a *lxcAction) Kind() string { return "pve.lxc" }

func (a *lxcAction) Ops() []spec.Op {
	cmd := pveCmd{id: a.id, step: a.step}

	createOp := &createLxcOp{
		pveCmd:        cmd,
		template:      a.template,
		hostname:      a.hostname,
		state:         a.state,
		cores:         a.cores,
		memoryMiB:     a.memoryMiB,
		swapMiB:       a.swapMiB,
		storage:       a.storage,
		sizeGiB:       a.sizeGiB,
		privileged:    a.privileged,
		network:       a.network,
		tags:          a.tags,
		sshPublicKeys: a.sshPublicKeys,
	}
	createOp.SetAction(a)

	// Absent: just create op (which handles destroy).
	if a.state == StateAbsent {
		return []spec.Op{createOp}
	}

	dlOp := &downloadTemplateOp{
		template: *a.template,
		step:     a.step,
	}
	dlOp.SetAction(a)
	createOp.AddDependency(dlOp)

	// Config, resize, SSH keys — run in parallel, all depend on create.
	cfgOp := &configLxcOp{
		pveCmd:     cmd,
		node:       a.node,
		hostname:   a.hostname,
		cores:      a.cores,
		memoryMiB:  a.memoryMiB,
		swapMiB:    a.swapMiB,
		storage:    a.storage,
		privileged: a.privileged,
		features:   a.features,
		startup:    a.startup,
		network:    a.network,
		tags:       a.tags,
	}
	cfgOp.SetAction(a)
	cfgOp.AddDependency(createOp)

	resizeOp := &resizeLxcOp{
		pveCmd:  cmd,
		sizeGiB: a.sizeGiB,
	}
	resizeOp.SetAction(a)
	resizeOp.AddDependency(createOp)

	keysOp := &sshKeysLxcOp{
		pveCmd:        cmd,
		sshPublicKeys: a.sshPublicKeys,
	}
	keysOp.SetAction(a)
	keysOp.AddDependency(createOp)

	// Reboot depends on config (hostname/features changes need reboot).
	rebootOp := &rebootLxcOp{
		pveCmd:   cmd,
		hostname: a.hostname,
		features: a.features,
	}
	rebootOp.SetAction(a)
	rebootOp.AddDependency(cfgOp)

	// State depends on config, resize, reboot — runs after those settle.
	stOp := &stateLxcOp{
		pveCmd: cmd,
		state:  a.state,
	}
	stOp.SetAction(a)
	stOp.AddDependency(cfgOp)
	stOp.AddDependency(resizeOp)
	stOp.AddDependency(rebootOp)

	// SSH keys need a running container (pct exec/push).
	// Depends on state so the container is started first.
	keysOp.AddDependency(stOp)

	return []spec.Op{dlOp, createOp, cfgOp, resizeOp, rebootOp, stOp, keysOp}
}
