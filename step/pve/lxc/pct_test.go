// SPDX-License-Identifier: GPL-3.0-only

package lxc

import "testing"

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
		id:        100,
		node:      "pve1",
		template:  LxcTemplate{Storage: "local", Name: "debian-12-standard_12.7-1_amd64.tar.zst"},
		hostname:  "pihole",
		cores:     2,
		memoryMiB: 512,
		storage:   "local-zfs",
		sizeGiB:   4,
		network:   LxcNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
	}

	got := buildCreateCmd(cfg)
	want := "pct create 100 local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst" +
		" --hostname pihole" +
		" --cores 2" +
		" --memory 512" +
		" --rootfs local-zfs:4" +
		" --net0 name=eth0,bridge=vmbr0,ip=10.10.10.10/24,gw=10.10.10.1,type=veth" +
		" --unprivileged 1" +
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

func TestBuildDownloadCmd(t *testing.T) {
	got := buildDownloadCmd("local", "debian-12-standard_12.7-1_amd64.tar.zst")
	want := "pveam download local debian-12-standard_12.7-1_amd64.tar.zst"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
