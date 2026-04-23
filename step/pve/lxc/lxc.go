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
		CPU           *LxcCPU      `step:"CPU configuration" optional:"true"`
		Memory        string       `step:"Memory with unit (e.g. 512M, 2G)" default:"512M"`
		Swap          string       `step:"Swap with unit — defaults to memory value" optional:"true"`
		Storage       string       `step:"Storage pool for rootfs" default:"local-zfs"`
		Size          string       `step:"Root disk size with unit (e.g. 8G, 500M)" default:"8G"`
		Privileged    bool         `step:"Run as privileged container (less secure)" default:"false"`
		DNS           *LxcDNS      `step:"DNS configuration" optional:"true"`
		Features      *LxcFeatures `step:"Advanced LXC features" optional:"true"`
		Startup       *LxcStartup  `step:"Startup/shutdown ordering" optional:"true"`
		Networks      []LxcNet     `step:"Network interfaces" optional:"true"`
		Devices       []LxcDevice  `step:"Device passthrough (PVE 8.1+)" optional:"true"`
		Tags          []string     `step:"PVE tags" optional:"true"`
		Password      string       `step:"Root password (create-only)" optional:"true"`
		SSHPublicKeys []string     `step:"SSH public keys for root" optional:"true"`
		Desc          string       `step:"Human-readable description" optional:"true"`
	}
	LxcTemplate struct {
		Storage string `step:"Storage pool holding the template" default:"local"`
		Name    string `step:"Template filename" example:"debian-12-standard_12.7-1_amd64.tar.zst"`
	}
	LxcNet struct {
		Name   string `step:"Interface name inside container" optional:"true"`
		Bridge string `step:"Bridge interface" default:"vmbr0"`
		IP     string `step:"IP address in CIDR or dhcp" example:"10.10.10.10/24"`
		Gw     string `step:"Gateway" optional:"true" example:"10.10.10.1"`
	}
	LxcDevice struct {
		Path string `step:"Host device path" example:"/dev/dri/renderD128"`
		Mode string `step:"Permission mode" default:"0666"`
	}
	LxcCPU struct {
		Cores  int    `step:"CPU cores" default:"1"`
		Limit  string `step:"CPU usage limit (e.g. 0.5, 2)" optional:"true"`
		Weight int    `step:"CPU weight for scheduler" optional:"true"`
	}
	LxcDNS struct {
		Nameserver   string `step:"DNS server IP" optional:"true"`
		Searchdomain string `step:"DNS search domain" optional:"true"`
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
		cpu:           resolveCPU(cfg.CPU),
		memoryMiB:     sizeToMiB(cfg.Memory),
		swapMiB:       resolveSwap(cfg.Swap, cfg.Memory),
		storage:       cfg.Storage,
		sizeGiB:       sizeToGiB(cfg.Size),
		privileged:    cfg.Privileged,
		dns:           resolveDNS(cfg.DNS),
		features:      cfg.Features,
		startup:       cfg.Startup,
		networks:      cfg.Networks,
		devices:       cfg.Devices,
		tags:          cfg.Tags,
		password:      cfg.Password,
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

func resolveCPU(c *LxcCPU) LxcCPU {
	if c == nil {
		return LxcCPU{Cores: 1}
	}
	if c.Cores == 0 {
		c.Cores = 1
	}
	return *c
}

func resolveDNS(d *LxcDNS) LxcDNS {
	if d == nil {
		return LxcDNS{}
	}
	return *d
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
	names := make(map[string]bool)
	for i, net := range c.Networks {
		if net.IP == "" {
			return InvalidConfigError{
				Field:  fmt.Sprintf("networks[%d].ip", i),
				Reason: "IP address is required",
				Source: step.Fields["networks"].Value,
			}
		}
		name := net.Name
		if name == "" {
			name = fmt.Sprintf("eth%d", i)
		}
		if names[name] {
			return InvalidConfigError{
				Field:  fmt.Sprintf("networks[%d].name", i),
				Reason: fmt.Sprintf("duplicate interface name %q", name),
				Source: step.Fields["networks"].Value,
			}
		}
		names[name] = true
	}
	devPaths := make(map[string]bool)
	for i, dev := range c.Devices {
		if dev.Path == "" {
			return InvalidConfigError{
				Field:  fmt.Sprintf("devices[%d].path", i),
				Reason: "device path is required",
				Source: step.Fields["devices"].Value,
			}
		}
		if !strings.HasPrefix(dev.Path, "/dev/") {
			return InvalidConfigError{
				Field:  fmt.Sprintf("devices[%d].path", i),
				Reason: fmt.Sprintf("device path %q must start with /dev/", dev.Path),
				Source: step.Fields["devices"].Value,
			}
		}
		if devPaths[dev.Path] {
			return InvalidConfigError{
				Field:  fmt.Sprintf("devices[%d].path", i),
				Reason: fmt.Sprintf("duplicate device path %q", dev.Path),
				Source: step.Fields["devices"].Value,
			}
		}
		devPaths[dev.Path] = true
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
	cpu           LxcCPU
	memoryMiB     int
	swapMiB       int
	storage       string
	sizeGiB       int
	privileged    bool
	dns           LxcDNS
	features      *LxcFeatures
	startup       *LxcStartup
	networks      []LxcNet
	devices       []LxcDevice
	tags          []string
	password      string
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
		cpu:           a.cpu,
		memoryMiB:     a.memoryMiB,
		swapMiB:       a.swapMiB,
		storage:       a.storage,
		sizeGiB:       a.sizeGiB,
		privileged:    a.privileged,
		networks:      a.networks,
		devices:       a.devices,
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

	// Scalar config (cores, memory, hostname, tags, etc.).
	cfgOp := &configLxcOp{
		pveCmd:     cmd,
		node:       a.node,
		hostname:   a.hostname,
		cpu:        a.cpu,
		memoryMiB:  a.memoryMiB,
		swapMiB:    a.swapMiB,
		storage:    a.storage,
		privileged: a.privileged,
		dns:        a.dns,
		features:   a.features,
		startup:    a.startup,
		tags:       a.tags,
	}
	cfgOp.SetAction(a)
	cfgOp.AddDependency(createOp)

	// Indexed config — each runs in parallel, all depend on create.
	netOp := &networkLxcOp{pveCmd: cmd, networks: a.networks}
	netOp.SetAction(a)
	netOp.AddDependency(createOp)

	devOp := &deviceLxcOp{pveCmd: cmd, devices: a.devices}
	devOp.SetAction(a)
	devOp.AddDependency(createOp)

	resizeOp := &resizeLxcOp{pveCmd: cmd, sizeGiB: a.sizeGiB}
	resizeOp.SetAction(a)
	resizeOp.AddDependency(createOp)

	keysOp := &sshKeysLxcOp{pveCmd: cmd, sshPublicKeys: a.sshPublicKeys}
	keysOp.SetAction(a)
	keysOp.AddDependency(createOp)

	// Reboot depends on all config ops (must run after they write
	// the conf file so the reboot picks up the new values).
	rebootOp := &rebootLxcOp{
		pveCmd:   cmd,
		hostname: a.hostname,
		features: a.features,
		dns:      a.dns,
		devices:  a.devices,
	}
	rebootOp.SetAction(a)
	rebootOp.AddDependency(cfgOp)
	rebootOp.AddDependency(devOp)

	// State depends on reboot + resize — runs after those settle.
	stOp := &stateLxcOp{pveCmd: cmd, state: a.state}
	stOp.SetAction(a)
	stOp.AddDependency(resizeOp)
	stOp.AddDependency(rebootOp)

	// SSH keys need a running container (pct exec/push).
	keysOp.AddDependency(stOp)

	return []spec.Op{
		dlOp, createOp,
		cfgOp, netOp, devOp, resizeOp,
		rebootOp, stOp, keysOp,
	}
}
