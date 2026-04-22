// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"strings"
	"testing"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

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

func TestFormatNet0(t *testing.T) {
	tests := []struct {
		name string
		net  LxcNet
		want string
	}{
		{
			name: "ip and gw",
			net:  LxcNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
			want: "name=eth0,bridge=vmbr0,ip=10.10.10.10/24,gw=10.10.10.1,type=veth",
		},
		{
			name: "ip only",
			net:  LxcNet{Bridge: "vmbr0", IP: "dhcp"},
			want: "name=eth0,bridge=vmbr0,ip=dhcp,type=veth",
		},
		{
			name: "custom bridge",
			net:  LxcNet{Bridge: "vmbr1", IP: "192.168.1.5/24", Gw: "192.168.1.1"},
			want: "name=eth0,bridge=vmbr1,ip=192.168.1.5/24,gw=192.168.1.1,type=veth",
		},
		{
			name: "empty bridge defaults to vmbr0",
			net:  LxcNet{IP: "10.0.0.1/24"},
			want: "name=eth0,bridge=vmbr0,ip=10.0.0.1/24,type=veth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatNet0(tt.net)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildCreateCmd(t *testing.T) {
	cfg := lxcAction{
		id:         100,
		node:       "pve1",
		template:   &LxcTemplate{Storage: "local", Name: "debian-12-standard_12.7-1_amd64.tar.zst"},
		hostname:   "pihole",
		cores:      2,
		memoryMiB:  512,
		swapMiB:    512,
		storage:    "local-zfs",
		sizeGiB:    4,
		privileged: true,
		network:    LxcNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
	}

	got := buildCreateCmd(cfg)
	want := "pct create 100 local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst" +
		" --hostname pihole" +
		" --cores 2" +
		" --memory 512" +
		" --swap 512" +
		" --rootfs local-zfs:4" +
		" --net0 name=eth0,bridge=vmbr0,ip=10.10.10.10/24,gw=10.10.10.1,type=veth" +
		" --unprivileged 0" +
		" --password yolo123"
	if got != want {
		t.Errorf("buildCreateCmd:\n got: %s\nwant: %s", got, want)
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
	if cfg.Net.Bridge != "vmbr0" {
		t.Errorf("net.bridge: got %q, want %q", cfg.Net.Bridge, "vmbr0")
	}
	if cfg.Net.IP != "10.10.10.10/24" {
		t.Errorf("net.ip: got %q, want %q", cfg.Net.IP, "10.10.10.10/24")
	}
	if cfg.Net.Gw != "10.10.10.1" {
		t.Errorf("net.gw: got %q, want %q", cfg.Net.Gw, "10.10.10.1")
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
				switch {
				case strings.Contains(cmd, "pct pull"):
					if tt.catFails {
						return target.CommandResult{ExitCode: 1, Stderr: "No such file"}, nil
					}
					return target.CommandResult{}, nil
				case strings.HasPrefix(cmd, "cat "):
					return target.CommandResult{Stdout: tt.catOutput}, nil
				case strings.HasPrefix(cmd, "rm "):
					return target.CommandResult{}, nil
				default:
					return target.CommandResult{ExitCode: 1}, nil
				}
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

	// But Check guards on status — stopped container → satisfied, skip key management.
	result, _, err := op.Check(context.Background(), nil, mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != spec.CheckSatisfied {
		t.Errorf("Check on stopped container: got %v, want CheckSatisfied", result)
	}
}

func TestBuildDownloadCmd(t *testing.T) {
	got := buildDownloadCmd("local", "debian-12-standard_12.7-1_amd64.tar.zst")
	want := "pveam download local debian-12-standard_12.7-1_amd64.tar.zst"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
