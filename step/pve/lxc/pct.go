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

// formatNet0 builds the --net0 value for pct create/set.
//
//	name=eth0,bridge=vmbr0,ip=10.10.10.10/24,gw=10.10.10.1,type=veth
func formatNet0(net LxcNet) string {
	var b strings.Builder
	b.WriteString("name=eth0")

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
	Memory       int
	Swap         int
	Unprivileged int    // 0 or 1
	Tags         string // semicolon-separated
	Description  string
	Storage      string // rootfs storage pool
	Size         string // rootfs size with unit (e.g. "4G")
	Net          parsedNet
}

type parsedNet struct {
	Bridge string
	IP     string
	Gw     string
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
		case "memory":
			cfg.Memory, _ = strconv.Atoi(val)
		case "swap":
			cfg.Swap, _ = strconv.Atoi(val)
		case "unprivileged":
			cfg.Unprivileged, _ = strconv.Atoi(val)
		case "tags":
			cfg.Tags = val
		case "description":
			cfg.Description = val
		case "rootfs":
			cfg.Storage, cfg.Size = parseRootfs(val)
		case "net0":
			cfg.Net = parseNet0Value(val)
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

// parseNet0Value parses the comma-separated key=value net0 config value.
//
//	"name=eth0,bridge=vmbr0,hwaddr=...,ip=10.10.10.10/24,gw=10.10.10.1,type=veth"
func parseNet0Value(val string) parsedNet {
	var net parsedNet
	for kv := range strings.SplitSeq(val, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch k {
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
		" --net0 %s"+
		" --unprivileged %d"+
		" --password yolo123",
		cfg.id, cfg.template.templatePath(),
		cfg.hostname,
		cfg.cores,
		cfg.memoryMiB,
		cfg.swapMiB,
		cfg.storage, cfg.sizeGiB,
		formatNet0(cfg.network),
		boolToInt(!cfg.privileged),
	)
	if len(cfg.tags) > 0 {
		cmd += " --tags " + shellQuote(strings.Join(cfg.tags, ";"))
	}
	if cfg.desc != "" {
		cmd += " --description " + shellQuote(cfg.desc)
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
