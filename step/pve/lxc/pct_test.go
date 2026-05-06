// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"strings"
	"testing"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

func TestParseResolvConfNameserver(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"only comments", "# auto-generated\n# by PVE\n", ""},
		{"single nameserver", "nameserver 1.1.1.1\n", "1.1.1.1"},
		{"with search and trailing newline", "search lan\nnameserver 10.0.0.1\n", "10.0.0.1"},
		// PVE encodes multiple nameservers as a single space-separated
		// string in `pct config` (e.g. `nameserver: 1.1.1.1 8.8.8.8`),
		// and the user-facing scampi config matches that shape:
		// `pve.Dns { nameserver = "1.1.1.1 8.8.8.8" }`. The probe of
		// /etc/resolv.conf must produce the same shape so the reboot
		// op's drift comparison doesn't loop forever (#283).
		{"multiple nameservers joined", "nameserver 1.1.1.1\nnameserver 8.8.8.8\n", "1.1.1.1 8.8.8.8"},
		{
			name:  "three nameservers",
			input: "nameserver 127.0.0.1\nnameserver 10.10.2.201\nnameserver 10.10.10.10\n",
			want:  "127.0.0.1 10.10.2.201 10.10.10.10",
		},
		{"comment before nameserver", "# header\nnameserver 9.9.9.9\n", "9.9.9.9"},
		{"no nameserver line", "search lan\noptions edns0\n", ""},
		{
			name:  "interleaved with comments and search",
			input: "search lan\nnameserver 1.1.1.1\n# notes\nnameserver 8.8.8.8\n",
			want:  "1.1.1.1 8.8.8.8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseResolvConfNameserver(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseResolvConfSearchdomain(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"single domain", "search lan\n", "lan"},
		{"multi-domain preserved", "search foo.com bar.com\nnameserver 1.1.1.1\n", "foo.com bar.com"},
		{"first wins", "search a.com\nsearch b.com\n", "a.com"},
		{"comment before search", "# header\nsearch lan\n", "lan"},
		{"no search line", "nameserver 1.1.1.1\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseResolvConfSearchdomain(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePctList(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantLen int
		checks  map[int]pctListEntry
	}{
		{
			name:    "empty",
			output:  "",
			wantLen: 0,
		},
		{
			name:    "header only",
			output:  "VMID       Status     Lock         Name\n",
			wantLen: 0,
		},
		{
			name: "two containers",
			output: `VMID       Status     Lock         Name
100        running                 pihole
101        stopped                 dns
`,
			wantLen: 2,
			checks: map[int]pctListEntry{
				100: {VMID: 100, Status: "running", Name: "pihole"},
				101: {VMID: 101, Status: "stopped", Name: "dns"},
			},
		},
		{
			name: "locked container",
			output: `VMID       Status     Lock         Name
200        stopped    backup       db
`,
			wantLen: 1,
			checks: map[int]pctListEntry{
				200: {VMID: 200, Status: "stopped", Name: "db"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePctList(tt.output)
			if len(got) != tt.wantLen {
				t.Fatalf("got %d entries, want %d", len(got), tt.wantLen)
			}
			for vmid, want := range tt.checks {
				entry, ok := got[vmid]
				if !ok {
					t.Fatalf("VMID %d not found", vmid)
				}
				if entry != want {
					t.Errorf("VMID %d: got %+v, want %+v", vmid, entry, want)
				}
			}
		})
	}
}

func TestParsePctStatus(t *testing.T) {
	tests := []struct {
		output string
		want   string
	}{
		{"status: running\n", "running"},
		{"status: stopped\n", "stopped"},
		{"status: running", "running"},
		{"garbage", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := parsePctStatus(tt.output)
		if got != tt.want {
			t.Errorf("parsePctStatus(%q) = %q, want %q", tt.output, got, tt.want)
		}
	}
}

func TestFormatNet(t *testing.T) {
	tests := []struct {
		name string
		idx  int
		net  LxcNet
		want string
	}{
		{
			name: "ip and gw",
			net:  LxcNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
			want: "name=eth0,bridge=vmbr0,ip=10.10.10.10/24,gw=10.10.10.1,type=veth",
		},
		{
			name: "custom bridge index 1",
			idx:  1,
			net:  LxcNet{Bridge: "vmbr1", IP: "192.168.1.5/24", Gw: "192.168.1.1"},
			want: "name=eth1,bridge=vmbr1,ip=192.168.1.5/24,gw=192.168.1.1,type=veth",
		},
		{
			name: "custom name",
			net:  LxcNet{Name: "mgmt", Bridge: "vmbr0", IP: "10.0.0.1/24"},
			want: "name=mgmt,bridge=vmbr0,ip=10.0.0.1/24,type=veth",
		},
		{
			name: "dhcp",
			net:  LxcNet{Bridge: "vmbr0", IP: "dhcp"},
			want: "name=eth0,bridge=vmbr0,ip=dhcp,type=veth",
		},
		{
			name: "vlan tag",
			net:  LxcNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1", VlanTag: 100},
			want: "name=eth0,bridge=vmbr0,ip=10.10.10.10/24,gw=10.10.10.1,tag=100,type=veth",
		},
		{
			name: "dhcp with vlan",
			net:  LxcNet{Bridge: "vmbr0", IP: "dhcp", VlanTag: 200},
			want: "name=eth0,bridge=vmbr0,ip=dhcp,tag=200,type=veth",
		},
		{
			name: "with pinned mac",
			net:  LxcNet{Bridge: "vmbr0", IP: "dhcp", Mac: "BE:EF:CA:FE:00:01"},
			want: "name=eth0,bridge=vmbr0,ip=dhcp,hwaddr=BE:EF:CA:FE:00:01,type=veth",
		},
		{
			name: "lowercase mac normalised to upper",
			net:  LxcNet{Bridge: "vmbr0", IP: "10.0.0.5/24", Mac: "be:ef:ca:fe:00:01"},
			want: "name=eth0,bridge=vmbr0,ip=10.0.0.5/24,hwaddr=BE:EF:CA:FE:00:01,type=veth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatNet(tt.idx, tt.net)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseNetValue_VlanTag(t *testing.T) {
	net := parseNetValue("name=eth0,bridge=vmbr0,ip=10.0.0.1/24,gw=10.0.0.1,tag=100,type=veth")
	if net.VlanTag != 100 {
		t.Errorf("tag = %d, want 100", net.VlanTag)
	}
	if net.IP != "10.0.0.1/24" {
		t.Errorf("ip = %q", net.IP)
	}
}

func TestParseNetValue_DHCP(t *testing.T) {
	net := parseNetValue("name=eth0,bridge=vmbr0,ip=dhcp,type=veth")
	if net.IP != "dhcp" {
		t.Errorf("ip = %q, want %q", net.IP, "dhcp")
	}
	if net.Gw != "" {
		t.Errorf("gw should be empty for DHCP, got %q", net.Gw)
	}
}

func TestParseNetValue_HwAddr(t *testing.T) {
	// PVE returns hwaddr in any case but our normalisation should
	// always store it uppercase, so a config with `mac = "be:ef:..."`
	// matches the parsed `BE:EF:...` and doesn't trigger drift.
	cases := []string{
		"name=eth0,bridge=vmbr0,hwaddr=BE:EF:CA:FE:00:01,ip=dhcp,type=veth",
		"name=eth0,bridge=vmbr0,hwaddr=be:ef:ca:fe:00:01,ip=dhcp,type=veth",
	}
	for _, in := range cases {
		net := parseNetValue(in)
		if net.Mac != "BE:EF:CA:FE:00:01" {
			t.Errorf("mac = %q, want uppercase BE:EF:CA:FE:00:01", net.Mac)
		}
	}
}

func TestNetRoundtrip_VlanTag(t *testing.T) {
	net := LxcNet{Bridge: "vmbr0", IP: "10.0.0.5/24", Gw: "10.0.0.1", VlanTag: 42}
	formatted := formatNet(0, net)
	parsed := parseNetValue(formatted)
	reparsed := parsedToLxcNet(parsed)
	reformatted := formatNet(0, reparsed)
	if formatted != reformatted {
		t.Errorf("roundtrip: %q → %q", formatted, reformatted)
	}
}

func TestBuildCreateCmd(t *testing.T) {
	cfg := lxcAction{
		id:         100,
		node:       "pve1",
		template:   &LxcTemplate{Storage: "local", Name: "debian-12-standard_12.7-1_amd64.tar.zst"},
		hostname:   "pihole",
		cpu:        LxcCPU{Cores: 2},
		memoryMiB:  512,
		swapMiB:    512,
		storage:    "local-zfs",
		sizeGiB:    4,
		privileged: true,
		networks:   []LxcNet{{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"}},
	}

	got := buildCreateCmd(cfg)
	want := "pct create 100 local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst" +
		" --hostname pihole" +
		" --cores 2" +
		" --memory 512" +
		" --swap 512" +
		" --rootfs local-zfs:4" +
		" --unprivileged 0" +
		" --net0 name=eth0,bridge=vmbr0,ip=10.10.10.10/24,gw=10.10.10.1,type=veth"
	if got != want {
		t.Errorf("buildCreateCmd:\n got: %s\nwant: %s", got, want)
	}

	// With password.
	cfg.password = "secret"
	got = buildCreateCmd(cfg)
	if !strings.Contains(got, "--password 'secret'") {
		t.Errorf("expected --password in command, got: %s", got)
	}

	// Without password — no --password flag.
	cfg.password = ""
	got = buildCreateCmd(cfg)
	if strings.Contains(got, "--password") {
		t.Errorf("unexpected --password in command, got: %s", got)
	}
}

func TestParsePctConfig(t *testing.T) {
	output := `arch: amd64
cores: 2
hostname: pihole
memory: 512
net0: name=eth0,bridge=vmbr0,hwaddr=BE:EF:00:00:01:00,ip=10.10.10.10/24,gw=10.10.10.1,type=veth
onboot: 1
ostype: debian
rootfs: local-zfs:subvol-999-disk-0,size=4G
swap: 512
unprivileged: 1
`
	cfg := parsePctConfig(output)

	if cfg.Cores != 2 {
		t.Errorf("cores: got %d, want 2", cfg.Cores)
	}
	if cfg.Memory != 512 {
		t.Errorf("memory: got %d, want 512", cfg.Memory)
	}
	if cfg.Hostname != "pihole" {
		t.Errorf("hostname: got %q, want %q", cfg.Hostname, "pihole")
	}
	if cfg.Storage != "local-zfs" {
		t.Errorf("storage: got %q, want %q", cfg.Storage, "local-zfs")
	}
	if cfg.Size != "4G" {
		t.Errorf("size: got %q, want %q", cfg.Size, "4G")
	}
	if len(cfg.Nets) != 1 {
		t.Fatalf("nets: got %d, want 1", len(cfg.Nets))
	}
	if cfg.Nets[0].Bridge != "vmbr0" {
		t.Errorf("net0.bridge: got %q, want %q", cfg.Nets[0].Bridge, "vmbr0")
	}
	if cfg.Nets[0].IP != "10.10.10.10/24" {
		t.Errorf("net0.ip: got %q, want %q", cfg.Nets[0].IP, "10.10.10.10/24")
	}
	if cfg.Nets[0].Gw != "10.10.10.1" {
		t.Errorf("net0.gw: got %q, want %q", cfg.Nets[0].Gw, "10.10.10.1")
	}
}

func TestParseRootfs(t *testing.T) {
	tests := []struct {
		val         string
		wantStorage string
		wantSize    string
	}{
		{"local-zfs:subvol-100-disk-0,size=4G", "local-zfs", "4G"},
		{"local-zfs:vm-100-disk-0,size=10G", "local-zfs", "10G"},
		{"ceph:subvol-200-disk-0,size=32G", "ceph", "32G"},
		{"local-lvm:vm-100-disk-0,size=8G", "local-lvm", "8G"},
	}
	for _, tt := range tests {
		storage, size := parseRootfs(tt.val)
		if storage != tt.wantStorage || size != tt.wantSize {
			t.Errorf("parseRootfs(%q) = (%q, %q), want (%q, %q)",
				tt.val, storage, size, tt.wantStorage, tt.wantSize)
		}
	}
}

func TestParseSizeGiB(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"4G", 4},
		{"10G", 10},
		{"4", 4},
		{"32g", 32},
	}
	for _, tt := range tests {
		got := parseSizeGiB(tt.input)
		if got != tt.want {
			t.Errorf("parseSizeGiB(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParsePVEKeys(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "standard PVE block",
			content: "# --- BEGIN PVE ---\nssh-rsa AAAA... user@host\nssh-ed25519 AAAA... other\n# --- END PVE ---\n",
			want:    []string{"ssh-rsa AAAA... user@host", "ssh-ed25519 AAAA... other"},
		},
		{
			name:    "empty PVE block",
			content: "# --- BEGIN PVE ---\n# --- END PVE ---\n",
			want:    nil,
		},
		{
			name:    "no PVE block",
			content: "ssh-rsa AAAA... manually-added\n",
			want:    nil,
		},
		{
			name: "PVE block with user keys outside",
			content: "ssh-rsa manual-key\n# --- BEGIN PVE ---\n" +
				"ssh-rsa managed-key\n# --- END PVE ---\nssh-ed25519 another-manual\n",
			want: []string{"ssh-rsa managed-key"},
		},
		{
			name:    "empty file",
			content: "",
			want:    nil,
		},
		{
			name:    "single key",
			content: "# --- BEGIN PVE ---\nssh-ed25519 AAAA... hal9000\n# --- END PVE ---\n",
			want:    []string{"ssh-ed25519 AAAA... hal9000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePVEKeys(tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d keys, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("key[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildAuthorizedKeys(t *testing.T) {
	t.Run("with keys", func(t *testing.T) {
		got := buildAuthorizedKeys([]string{"ssh-rsa AAAA...", "ssh-ed25519 BBBB..."})
		want := "# --- BEGIN PVE ---\nssh-rsa AAAA...\nssh-ed25519 BBBB...\n# --- END PVE ---\n"
		if got != want {
			t.Errorf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := buildAuthorizedKeys(nil)
		want := "# --- BEGIN PVE ---\n# --- END PVE ---\n"
		if got != want {
			t.Errorf("got:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		keys := []string{"ssh-rsa AAAA...", "ssh-ed25519 BBBB..."}
		content := buildAuthorizedKeys(keys)
		parsed := parsePVEKeys(content)
		if len(parsed) != len(keys) {
			t.Fatalf("roundtrip: got %d keys, want %d", len(parsed), len(keys))
		}
		for i := range parsed {
			if parsed[i] != keys[i] {
				t.Errorf("roundtrip key[%d]: got %q, want %q", i, parsed[i], keys[i])
			}
		}
	})
}

func TestSSHKeyDrift(t *testing.T) {
	tests := []struct {
		name      string
		desired   []string
		catOutput string
		catFails  bool
		wantDrift bool
	}{
		{
			name:      "no keys desired, no file",
			desired:   nil,
			catFails:  true,
			wantDrift: false,
		},
		{
			name:      "keys desired, no file",
			desired:   []string{"ssh-rsa AAAA..."},
			catFails:  true,
			wantDrift: true,
		},
		{
			name:      "keys match",
			desired:   []string{"ssh-rsa AAAA..."},
			catOutput: "# --- BEGIN PVE ---\nssh-rsa AAAA...\n# --- END PVE ---\n",
			wantDrift: false,
		},
		{
			name:      "keys differ",
			desired:   []string{"ssh-ed25519 NEW..."},
			catOutput: "# --- BEGIN PVE ---\nssh-rsa OLD...\n# --- END PVE ---\n",
			wantDrift: true,
		},
		{
			name:      "no keys desired, file has PVE keys",
			desired:   nil,
			catOutput: "# --- BEGIN PVE ---\nssh-rsa LEFTOVER...\n# --- END PVE ---\n",
			wantDrift: true,
		},
		{
			name:      "no keys desired, file has no PVE section",
			desired:   nil,
			catOutput: "ssh-rsa manual-only\n",
			wantDrift: false,
		},
		{
			name:      "extra key added",
			desired:   []string{"ssh-rsa A", "ssh-rsa B"},
			catOutput: "# --- BEGIN PVE ---\nssh-rsa A\n# --- END PVE ---\n",
			wantDrift: true,
		},
		{
			name:      "key removed",
			desired:   []string{"ssh-rsa A"},
			catOutput: "# --- BEGIN PVE ---\nssh-rsa A\nssh-rsa B\n# --- END PVE ---\n",
			wantDrift: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdr := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
				if strings.HasPrefix(cmd, "pct exec 100 -- cat /root/.ssh/authorized_keys") {
					if tt.catFails {
						return target.CommandResult{ExitCode: 1}, nil
					}
					return target.CommandResult{Stdout: tt.catOutput}, nil
				}
				return target.CommandResult{ExitCode: 1}, nil
			}}

			op := &sshKeysLxcOp{pveCmd: pveCmd{id: 100}, sshPublicKeys: tt.desired}
			d := op.sshKeyDrift(context.Background(), cmdr)
			if tt.wantDrift && d == nil {
				t.Error("expected drift, got nil")
			}
			if !tt.wantDrift && d != nil {
				t.Errorf("expected no drift, got %+v", d)
			}
		})
	}
}

func TestSSHKeyDrift_StoppedContainer(t *testing.T) {
	mock := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
		switch {
		case strings.Contains(cmd, "pct list"):
			return target.CommandResult{
				Stdout: "VMID       Status     Lock         Name\n" +
					"100        stopped                 test\n",
			}, nil
		case cmd == "pct status 100":
			return target.CommandResult{Stdout: "status: stopped\n"}, nil
		default:
			return target.CommandResult{ExitCode: 1}, nil
		}
	}}

	op := &sshKeysLxcOp{
		pveCmd:        pveCmd{id: 100},
		sshPublicKeys: []string{"ssh-rsa AAAA..."},
	}

	// sshKeyDrift itself doesn't know about status — pct pull fails,
	// keys are desired → it reports drift. This is the bug surface.
	d := op.sshKeyDrift(context.Background(), mock)
	if d == nil {
		t.Fatal("sshKeyDrift should report drift when pull fails and keys desired")
	}

	// Check guards on status — stopped container → satisfied + warning.
	result, _, err := op.Check(context.Background(), nil, mock)
	if result != spec.CheckSatisfied {
		t.Errorf("Check on stopped container: got %v, want CheckSatisfied", result)
	}
	if err == nil {
		t.Fatal("expected SSHKeysSkippedWarning, got nil")
	}
	if _, ok := err.(SSHKeysSkippedWarning); !ok {
		t.Fatalf("expected SSHKeysSkippedWarning, got %T: %v", err, err)
	}
}

func TestFormatFeatures(t *testing.T) {
	tests := []struct {
		name string
		feat *LxcFeatures
		want string
	}{
		{"nil", nil, ""},
		{"empty", &LxcFeatures{}, ""},
		{"nesting only", &LxcFeatures{Nesting: true}, "nesting=1"},
		{
			"multiple bools",
			&LxcFeatures{Nesting: true, Keyctl: true, Fuse: true},
			"nesting=1,keyctl=1,fuse=1",
		},
		{"mount only", &LxcFeatures{Mount: []string{"nfs", "cifs"}}, "mount=nfs;cifs"},
		{
			"everything",
			&LxcFeatures{
				Nesting: true, Keyctl: true, Fuse: true,
				Mknod: true, ForceRwSys: true, Mount: []string{"nfs"},
			},
			"nesting=1,keyctl=1,fuse=1,mknod=1,force_rw_sys=1,mount=nfs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFeatures(tt.feat)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseFeatures(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want LxcFeatures
	}{
		{"empty", "", LxcFeatures{}},
		{"nesting", "nesting=1", LxcFeatures{Nesting: true}},
		{"multiple", "nesting=1,keyctl=1", LxcFeatures{Nesting: true, Keyctl: true}},
		{"mount", "mount=nfs;cifs", LxcFeatures{Mount: []string{"nfs", "cifs"}}},
		{
			"full",
			"nesting=1,keyctl=1,fuse=1,mknod=1,force_rw_sys=1,mount=nfs",
			LxcFeatures{
				Nesting: true, Keyctl: true, Fuse: true,
				Mknod: true, ForceRwSys: true, Mount: []string{"nfs"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFeatures(tt.val)
			if formatFeatures(&got) != formatFeatures(&tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFormatStartup(t *testing.T) {
	tests := []struct {
		name string
		s    *LxcStartup
		want string
	}{
		{"nil", nil, ""},
		{"empty", &LxcStartup{}, ""},
		{"order only", &LxcStartup{Order: 5}, "order=5"},
		{"all fields", &LxcStartup{Order: 1, Up: 30, Down: 60}, "order=1,up=30,down=60"},
		{"up only", &LxcStartup{Up: 10}, "up=10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatStartup(tt.s)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseStartup(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want LxcStartup
	}{
		{"empty", "", LxcStartup{}},
		{"order", "order=5", LxcStartup{Order: 5}},
		{"all", "order=1,up=30,down=60", LxcStartup{Order: 1, Up: 30, Down: 60}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStartup(tt.val)
			if formatStartup(&got) != formatStartup(&tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStartupRoundtrip(t *testing.T) {
	orig := LxcStartup{Order: 3, Up: 15, Down: 30}
	formatted := formatStartup(&orig)
	parsed := parseStartup(formatted)
	if formatStartup(&parsed) != formatted {
		t.Errorf("roundtrip failed: %q → %+v → %q",
			formatted, parsed, formatStartup(&parsed))
	}
}

func TestFeaturesRoundtrip(t *testing.T) {
	orig := LxcFeatures{
		Nesting: true,
		Keyctl:  true,
		Mount:   []string{"nfs", "cifs"},
	}
	formatted := formatFeatures(&orig)
	parsed := parseFeatures(formatted)
	if formatFeatures(&parsed) != formatted {
		t.Errorf(
			"roundtrip failed: %q → %+v → %q",
			formatted, parsed, formatFeatures(&parsed),
		)
	}
}

func TestBuildDownloadCmd(t *testing.T) {
	got := buildDownloadCmd("local", "debian-12-standard_12.7-1_amd64.tar.zst")
	want := "pveam download local debian-12-standard_12.7-1_amd64.tar.zst"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Device parse/format tests
// -----------------------------------------------------------------------------

func TestParseDevKey(t *testing.T) {
	tests := []struct {
		key     string
		wantIdx int
		wantOk  bool
	}{
		{"dev0", 0, true},
		{"dev1", 1, true},
		{"dev12", 12, true},
		{"net0", 0, false},
		{"dev", 0, false},
		{"devX", 0, false},
	}
	for _, tt := range tests {
		idx, ok := parseDevKey(tt.key)
		if ok != tt.wantOk || idx != tt.wantIdx {
			t.Errorf("parseDevKey(%q) = (%d, %v), want (%d, %v)",
				tt.key, idx, ok, tt.wantIdx, tt.wantOk)
		}
	}
}

func TestParseDevValue(t *testing.T) {
	tests := []struct {
		val  string
		want parsedDev
	}{
		{
			"/dev/dri/renderD128",
			parsedDev{Path: "/dev/dri/renderD128"},
		},
		{
			"/dev/dri/renderD128,mode=0666",
			parsedDev{Path: "/dev/dri/renderD128", Mode: "0666"},
		},
		{
			"/dev/kfd,mode=0660",
			parsedDev{Path: "/dev/kfd", Mode: "0660"},
		},
		{
			"/dev/nvidia0",
			parsedDev{Path: "/dev/nvidia0"},
		},
	}
	for _, tt := range tests {
		got := parseDevValue(tt.val)
		if got != tt.want {
			t.Errorf("parseDevValue(%q) = %+v, want %+v",
				tt.val, got, tt.want)
		}
	}
}

func TestFormatDev(t *testing.T) {
	tests := []struct {
		dev  LxcDevice
		want string
	}{
		{
			LxcDevice{Path: "/dev/dri/renderD128"},
			"/dev/dri/renderD128,mode=0666",
		},
		{
			LxcDevice{Path: "/dev/dri/renderD128", Mode: "0666"},
			"/dev/dri/renderD128,mode=0666",
		},
		{
			LxcDevice{Path: "/dev/kfd", Mode: "0660"},
			"/dev/kfd,mode=0660",
		},
	}
	for _, tt := range tests {
		got := formatDev(tt.dev)
		if got != tt.want {
			t.Errorf("formatDev(%+v) = %q, want %q",
				tt.dev, got, tt.want)
		}
	}
}

func TestDeviceRoundtrip(t *testing.T) {
	cases := []string{
		"/dev/dri/renderD128,mode=0666",
		"/dev/kfd,mode=0660",
		"/dev/nvidia0,mode=0666",
	}
	for _, val := range cases {
		parsed := parseDevValue(val)
		dev := parsedToLxcDevice(parsed)
		formatted := formatDev(dev)
		if formatted != val {
			t.Errorf("roundtrip %q → %+v → %q", val, dev, formatted)
		}
	}
}

func TestParsePctConfigDevices(t *testing.T) {
	output := "arch: amd64\n" +
		"cores: 2\n" +
		"dev0: /dev/dri/renderD128,mode=0666\n" +
		"dev1: /dev/kfd,mode=0660\n" +
		"hostname: gpu-box\n"
	cfg := parsePctConfig(output)
	if len(cfg.Devs) != 2 {
		t.Fatalf("got %d devs, want 2", len(cfg.Devs))
	}
	if cfg.Devs[0].Path != "/dev/dri/renderD128" {
		t.Errorf("dev0 path = %q", cfg.Devs[0].Path)
	}
	if cfg.Devs[1].Path != "/dev/kfd" {
		t.Errorf("dev1 path = %q", cfg.Devs[1].Path)
	}
}
