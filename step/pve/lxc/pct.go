// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"fmt"
	"strconv"
	"strings"

	"scampi.dev/scampi/spec"
)

// pctListEntry represents one row from `pct list` output.
type pctListEntry struct {
	VMID   int
	Status string
	Name   string
}

// parsePctList parses the tabular output of `pct list`.
//
//	VMID       Status     Lock         Name
//	100        running                 pihole
//	101        stopped                 dns
func parsePctList(output string) map[int]pctListEntry {
	entries := make(map[int]pctListEntry)
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "VMID") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		vmid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		// Fields: VMID, Status, [Lock], Name
		// Lock column may be empty — name is always last.
		entries[vmid] = pctListEntry{
			VMID:   vmid,
			Status: fields[1],
			Name:   fields[len(fields)-1],
		}
	}
	return entries
}

// parsePctStatus parses `pct status <id>` output.
// Expected format: "status: running" or "status: stopped"
func parsePctStatus(output string) string {
	_, status, ok := strings.Cut(strings.TrimSpace(output), ": ")
	if !ok {
		return ""
	}
	return status
}

// formatNet builds the --netN value for pct create/set.
//
//	name=eth0,bridge=vmbr0,ip=10.10.10.10/24,gw=10.10.10.1,type=veth
func formatNet(idx int, net LxcNet) string {
	var b strings.Builder

	name := net.Name
	if name == "" {
		name = fmt.Sprintf("eth%d", idx)
	}
	b.WriteString("name=")
	b.WriteString(name)

	bridge := net.Bridge
	if bridge == "" {
		bridge = "vmbr0"
	}
	b.WriteString(",bridge=")
	b.WriteString(bridge)

	b.WriteString(",ip=")
	b.WriteString(net.IP)

	if net.Gw != "" {
		b.WriteString(",gw=")
		b.WriteString(net.Gw)
	}

	b.WriteString(",type=veth")
	return b.String()
}

// pctConfig holds the parsed fields from `pct config <id>` that we care about.
type pctConfig struct {
	Hostname     string
	Cores        int
	CPULimit     string // "0.500000" — PVE stores with trailing zeros
	CPUUnits     int
	Memory       int
	Swap         int
	Unprivileged int // 0 or 1
	OnBoot       int // 0 or 1
	Startup      LxcStartup
	Features     LxcFeatures
	Nameserver   string
	Searchdomain string
	Tags         string // semicolon-separated
	Description  string
	Storage      string      // rootfs storage pool
	Size         string      // rootfs size with unit (e.g. "4G")
	Nets         []parsedNet // indexed by net0, net1, ...
	Devs         []parsedDev // indexed by dev0, dev1, ...
}

type parsedNet struct {
	Name   string
	Bridge string
	IP     string
	Gw     string
}

type parsedDev struct {
	Path string
	Mode string
}

// parseNetKey extracts the index from "net0", "net1", etc.
func parseNetKey(key string) (int, bool) {
	if !strings.HasPrefix(key, "net") {
		return 0, false
	}
	idx, err := strconv.Atoi(key[3:])
	if err != nil {
		return 0, false
	}
	return idx, true
}

// parseDevKey extracts the index from "dev0", "dev1", etc.
func parseDevKey(key string) (int, bool) {
	if !strings.HasPrefix(key, "dev") {
		return 0, false
	}
	idx, err := strconv.Atoi(key[3:])
	if err != nil {
		return 0, false
	}
	return idx, true
}

// parseDevValue parses a PVE device config value.
//
//	"/dev/dri/renderD128,mode=0666"
//	"/dev/kfd"
func parseDevValue(val string) parsedDev {
	var dev parsedDev
	parts := strings.SplitN(val, ",", 2)
	dev.Path = strings.TrimSpace(parts[0])
	if len(parts) < 2 {
		return dev
	}
	for kv := range strings.SplitSeq(parts[1], ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if k == "mode" {
			dev.Mode = v
		}
	}
	return dev
}

// formatDev builds the --devN value for pct create/set.
//
//	"/dev/dri/renderD128,mode=0666"
func formatDev(dev LxcDevice) string {
	var b strings.Builder
	b.WriteString(dev.Path)
	mode := dev.Mode
	if mode == "" {
		mode = "0666"
	}
	b.WriteString(",mode=")
	b.WriteString(mode)
	return b.String()
}

// devicesFingerprint produces a canonical string for device list comparison.
// Returns "(none)" for empty lists so the reboot check runner's
// empty-string guard (which means "probe failed") doesn't swallow it.
func devicesFingerprint(devs []LxcDevice) string {
	if len(devs) == 0 {
		return "(none)"
	}
	var parts []string
	for i, d := range devs {
		parts = append(parts, fmt.Sprintf("dev%d=%s", i, formatDev(d)))
	}
	return strings.Join(parts, ";")
}

// parsePctConfig parses the key: value output of `pct config <id>`.
//
//	arch: amd64
//	cores: 2
//	hostname: pihole
//	memory: 512
//	net0: name=eth0,bridge=vmbr0,ip=10.10.10.10/24,gw=10.10.10.1,type=veth
//	rootfs: local-zfs:subvol-100-disk-0,size=4G
func parsePctConfig(output string) pctConfig {
	var cfg pctConfig
	for line := range strings.SplitSeq(output, "\n") {
		key, val, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		switch key {
		case "hostname":
			cfg.Hostname = val
		case "cores":
			cfg.Cores, _ = strconv.Atoi(val)
		case "cpulimit":
			cfg.CPULimit = val
		case "cpuunits":
			cfg.CPUUnits, _ = strconv.Atoi(val)
		case "memory":
			cfg.Memory, _ = strconv.Atoi(val)
		case "swap":
			cfg.Swap, _ = strconv.Atoi(val)
		case "unprivileged":
			cfg.Unprivileged, _ = strconv.Atoi(val)
		case "nameserver":
			cfg.Nameserver = val
		case "searchdomain":
			cfg.Searchdomain = val
		case "tags":
			cfg.Tags = val
		case "description":
			cfg.Description = val
		case "onboot":
			cfg.OnBoot, _ = strconv.Atoi(val)
		case "startup":
			cfg.Startup = parseStartup(val)
		case "features":
			cfg.Features = parseFeatures(val)
		case "rootfs":
			cfg.Storage, cfg.Size = parseRootfs(val)
		default:
			if idx, ok := parseNetKey(key); ok {
				for len(cfg.Nets) <= idx {
					cfg.Nets = append(cfg.Nets, parsedNet{})
				}
				cfg.Nets[idx] = parseNetValue(val)
			} else if idx, ok := parseDevKey(key); ok {
				for len(cfg.Devs) <= idx {
					cfg.Devs = append(cfg.Devs, parsedDev{})
				}
				cfg.Devs[idx] = parseDevValue(val)
			}
		}
	}
	return cfg
}

// parseRootfs extracts storage pool and size from a rootfs value.
//
//	"local-zfs:subvol-100-disk-0,size=4G" → ("local-zfs", "4G")
//	"local-zfs:vm-100-disk-0,size=10G" → ("local-zfs", "10G")
func parseRootfs(val string) (storage, size string) {
	// Storage is before the first colon.
	storage, rest, ok := strings.Cut(val, ":")
	if !ok {
		return val, ""
	}
	// Size is in the comma-separated params after the volume name.
	for param := range strings.SplitSeq(rest, ",") {
		if k, v, ok := strings.Cut(param, "="); ok && k == "size" {
			size = v
			break
		}
	}
	return storage, size
}

// parseNetValue parses the comma-separated key=value netN config value.
//
//	"name=eth0,bridge=vmbr0,hwaddr=...,ip=10.10.10.10/24,gw=10.10.10.1,type=veth"
func parseNetValue(val string) parsedNet {
	var net parsedNet
	for kv := range strings.SplitSeq(val, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch k {
		case "name":
			net.Name = v
		case "bridge":
			net.Bridge = v
		case "ip":
			net.IP = v
		case "gw":
			net.Gw = v
		}
	}
	return net
}

// buildSetCmd builds a `pct set` command for changed mutable fields.
func buildSetCmd(vmid int, drift []spec.DriftDetail) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "pct set %d", vmid)
	for _, d := range drift {
		switch d.Field {
		case "cores":
			b.WriteString(" --cores ")
			b.WriteString(d.Desired)
		case "cpulimit":
			if d.Desired == "" || d.Desired == "(none)" {
				b.WriteString(" --delete cpulimit")
			} else {
				b.WriteString(" --cpulimit ")
				b.WriteString(d.Desired)
			}
		case "cpuunits":
			if d.Desired == "0" {
				b.WriteString(" --delete cpuunits")
			} else {
				b.WriteString(" --cpuunits ")
				b.WriteString(d.Desired)
			}
		case "memory":
			b.WriteString(" --memory ")
			b.WriteString(d.Desired)
		case "swap":
			b.WriteString(" --swap ")
			b.WriteString(d.Desired)
		case "hostname":
			b.WriteString(" --hostname ")
			b.WriteString(shellQuote(d.Desired))
		case "tags":
			b.WriteString(" --tags ")
			b.WriteString(shellQuote(d.Desired))
		case "description":
			b.WriteString(" --description ")
			b.WriteString(shellQuote(d.Desired))
		case "nameserver":
			if d.Desired == "" || d.Desired == "(none)" {
				b.WriteString(" --delete nameserver")
			} else {
				b.WriteString(" --nameserver ")
				b.WriteString(shellQuote(d.Desired))
			}
		case "searchdomain":
			if d.Desired == "" || d.Desired == "(none)" {
				b.WriteString(" --delete searchdomain")
			} else {
				b.WriteString(" --searchdomain ")
				b.WriteString(shellQuote(d.Desired))
			}
		case "features":
			b.WriteString(" --features ")
			b.WriteString(shellQuote(d.Desired))
		case "onboot":
			b.WriteString(" --onboot ")
			b.WriteString(d.Desired)
		case "startup":
			if d.Desired == "" || d.Desired == "(none)" {
				b.WriteString(" --delete startup")
			} else {
				b.WriteString(" --startup ")
				b.WriteString(shellQuote(d.Desired))
			}
		}
	}
	return b.String()
}

// buildCreateCmd builds the full `pct create` command.
// Template storage and rootfs storage are independent pools.
func buildCreateCmd(cfg lxcAction) string {
	cmd := fmt.Sprintf("pct create %d %s"+
		" --hostname %s"+
		" --cores %d"+
		" --memory %d"+
		" --swap %d"+
		" --rootfs %s:%d"+
		" --unprivileged %d",
		cfg.id, cfg.template.templatePath(),
		cfg.hostname,
		cfg.cpu.Cores,
		cfg.memoryMiB,
		cfg.swapMiB,
		cfg.storage, cfg.sizeGiB,
		boolToInt(!cfg.privileged),
	)
	if cfg.cpu.Limit != "" {
		cmd += " --cpulimit " + cfg.cpu.Limit
	}
	if cfg.cpu.Weight != 0 {
		cmd += fmt.Sprintf(" --cpuunits %d", cfg.cpu.Weight)
	}
	for i, net := range cfg.networks {
		cmd += fmt.Sprintf(" --net%d %s", i, formatNet(i, net))
	}
	if cfg.password != "" {
		cmd += " --password " + shellQuote(cfg.password)
	}
	if len(cfg.tags) > 0 {
		cmd += " --tags " + shellQuote(strings.Join(cfg.tags, ";"))
	}
	if cfg.desc != "" {
		cmd += " --description " + shellQuote(cfg.desc)
	}
	if cfg.dns.Nameserver != "" {
		cmd += " --nameserver " + shellQuote(cfg.dns.Nameserver)
	}
	if cfg.dns.Searchdomain != "" {
		cmd += " --searchdomain " + shellQuote(cfg.dns.Searchdomain)
	}
	if feat := formatFeatures(cfg.features); feat != "" {
		cmd += " --features " + shellQuote(feat)
	}
	if cfg.startup != nil {
		if cfg.startup.OnBoot {
			cmd += " --onboot 1"
		}
		if s := formatStartup(cfg.startup); s != "" {
			cmd += " --startup " + shellQuote(s)
		}
	}
	for i, dev := range cfg.devices {
		cmd += fmt.Sprintf(" --dev%d %s", i, formatDev(dev))
	}
	return cmd
}

// parsePVEKeys extracts SSH keys from the PVE-managed section of authorized_keys.
// Returns the keys between "# --- BEGIN PVE ---" and "# --- END PVE ---".
func parsePVEKeys(content string) []string {
	var keys []string
	inPVE := false
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "# --- BEGIN PVE ---" {
			inPVE = true
			continue
		}
		if line == "# --- END PVE ---" {
			break
		}
		if inPVE && line != "" {
			keys = append(keys, line)
		}
	}
	return keys
}

// buildAuthorizedKeys builds the PVE-managed authorized_keys content.
func buildAuthorizedKeys(keys []string) string {
	var b strings.Builder
	b.WriteString("# --- BEGIN PVE ---\n")
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('\n')
	}
	b.WriteString("# --- END PVE ---\n")
	return b.String()
}

// formatStartup builds the --startup value for pct create/set.
//
//	"order=1,up=30,down=60"
func formatStartup(s *LxcStartup) string {
	if s == nil {
		return ""
	}
	var parts []string
	if s.Order != 0 {
		parts = append(parts, fmt.Sprintf("order=%d", s.Order))
	}
	if s.Up != 0 {
		parts = append(parts, fmt.Sprintf("up=%d", s.Up))
	}
	if s.Down != 0 {
		parts = append(parts, fmt.Sprintf("down=%d", s.Down))
	}
	return strings.Join(parts, ",")
}

// parseStartup parses the startup value from pct config.
func parseStartup(val string) LxcStartup {
	var s LxcStartup
	for kv := range strings.SplitSeq(val, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch k {
		case "order":
			s.Order, _ = strconv.Atoi(v)
		case "up":
			s.Up, _ = strconv.Atoi(v)
		case "down":
			s.Down, _ = strconv.Atoi(v)
		}
	}
	return s
}

// formatFeatures builds the --features value for pct create/set.
//
//	"nesting=1,keyctl=1,mount=nfs;cifs"
func formatFeatures(f *LxcFeatures) string {
	if f == nil {
		return ""
	}
	var parts []string
	if f.Nesting {
		parts = append(parts, "nesting=1")
	}
	if f.Keyctl {
		parts = append(parts, "keyctl=1")
	}
	if f.Fuse {
		parts = append(parts, "fuse=1")
	}
	if f.Mknod {
		parts = append(parts, "mknod=1")
	}
	if f.ForceRwSys {
		parts = append(parts, "force_rw_sys=1")
	}
	if len(f.Mount) > 0 {
		parts = append(parts, "mount="+strings.Join(f.Mount, ";"))
	}
	return strings.Join(parts, ",")
}

// parseFeatures parses the features value from pct config.
func parseFeatures(val string) LxcFeatures {
	var f LxcFeatures
	for kv := range strings.SplitSeq(val, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch k {
		case "nesting":
			f.Nesting = v == "1"
		case "keyctl":
			f.Keyctl = v == "1"
		case "fuse":
			f.Fuse = v == "1"
		case "mknod":
			f.Mknod = v == "1"
		case "force_rw_sys":
			f.ForceRwSys = v == "1"
		case "mount":
			for fs := range strings.SplitSeq(v, ";") {
				if fs != "" {
					f.Mount = append(f.Mount, fs)
				}
			}
		}
	}
	return f
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// buildDownloadCmd builds the `pveam download` command from a parsed template.
func buildDownloadCmd(storage, filename string) string {
	return fmt.Sprintf("pveam download %s %s", storage, filename)
}
