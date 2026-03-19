// SPDX-License-Identifier: GPL-3.0-only

package pkgmgr

import (
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantID       string
		wantLike     []string
		wantCodename string
	}{
		{
			name: "ubuntu",
			input: `NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 22.04.3 LTS"
VERSION_ID="22.04"
VERSION_CODENAME=jammy
`,
			wantID:       "ubuntu",
			wantLike:     []string{"debian"},
			wantCodename: "jammy",
		},
		{
			name: "alpine",
			input: `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.19.0
PRETTY_NAME="Alpine Linux v3.19"
`,
			wantID:   "alpine",
			wantLike: nil,
		},
		{
			name: "rocky linux",
			input: `NAME="Rocky Linux"
VERSION="9.3 (Blue Onyx)"
ID="rocky"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.3"
PRETTY_NAME="Rocky Linux 9.3 (Blue Onyx)"
`,
			wantID:   "rocky",
			wantLike: []string{"rhel", "centos", "fedora"},
		},
		{
			name: "fedora",
			input: `NAME="Fedora Linux"
VERSION="39 (Workstation Edition)"
ID=fedora
VERSION_ID=39
PRETTY_NAME="Fedora Linux 39 (Workstation Edition)"
`,
			wantID:   "fedora",
			wantLike: nil,
		},
		{
			name: "arch",
			input: `NAME="Arch Linux"
PRETTY_NAME="Arch Linux"
ID=arch
BUILD_ID=rolling
`,
			wantID:   "arch",
			wantLike: nil,
		},
		{
			name: "opensuse",
			input: `NAME="openSUSE Leap"
VERSION="15.5"
ID="opensuse-leap"
ID_LIKE="suse opensuse"
PRETTY_NAME="openSUSE Leap 15.5"
`,
			wantID:   "opensuse-leap",
			wantLike: []string{"suse", "opensuse"},
		},
		{
			name:     "empty",
			input:    "",
			wantID:   "",
			wantLike: nil,
		},
		{
			name:     "comments only",
			input:    "# this is a comment\n# another comment\n",
			wantID:   "",
			wantLike: nil,
		},
		{
			name: "debian with codename",
			input: `ID=debian
ID_LIKE=""
VERSION_CODENAME=bookworm
VERSION_ID="12"
`,
			wantID:       "debian",
			wantCodename: "bookworm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ParseOSRelease([]byte(tt.input))
			if info.ID != tt.wantID {
				t.Errorf("ID: got %q, want %q", info.ID, tt.wantID)
			}
			if !stringSliceEqual(info.IDLike, tt.wantLike) {
				t.Errorf("IDLike: got %v, want %v", info.IDLike, tt.wantLike)
			}
			if info.VersionCodename != tt.wantCodename {
				t.Errorf("VersionCodename: got %q, want %q", info.VersionCodename, tt.wantCodename)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		info     OSInfo
		wantKind Kind // zero value means nil backend
	}{
		{
			name:     "darwin",
			info:     OSInfo{Kernel: KernelDarwin},
			wantKind: Brew,
		},
		{
			name:     "freebsd",
			info:     OSInfo{Kernel: KernelFreeBSD},
			wantKind: Pkg,
		},
		{
			name:     "debian",
			info:     OSInfo{Kernel: KernelLinux, ID: "debian"},
			wantKind: Apt,
		},
		{
			name:     "ubuntu direct",
			info:     OSInfo{Kernel: KernelLinux, ID: "ubuntu", IDLike: []string{"debian"}},
			wantKind: Apt,
		},
		{
			name:     "alpine",
			info:     OSInfo{Kernel: KernelLinux, ID: "alpine"},
			wantKind: Apk,
		},
		{
			name:     "fedora",
			info:     OSInfo{Kernel: KernelLinux, ID: "fedora"},
			wantKind: Dnf,
		},
		{
			name:     "arch",
			info:     OSInfo{Kernel: KernelLinux, ID: "arch"},
			wantKind: Pacman,
		},
		{
			name:     "rhel",
			info:     OSInfo{Kernel: KernelLinux, ID: "rhel"},
			wantKind: Dnf,
		},
		{
			name:     "suse",
			info:     OSInfo{Kernel: KernelLinux, ID: "suse"},
			wantKind: Zypper,
		},
		{
			name:     "ID_LIKE fallback ubuntu to debian",
			info:     OSInfo{Kernel: KernelLinux, ID: "linuxmint", IDLike: []string{"debian", "ubuntu"}},
			wantKind: Apt,
		},
		{
			name:     "ID_LIKE fallback rocky to rhel",
			info:     OSInfo{Kernel: KernelLinux, ID: "rocky", IDLike: []string{"rhel", "centos", "fedora"}},
			wantKind: Dnf,
		},
		{
			name:     "ID_LIKE fallback opensuse to suse",
			info:     OSInfo{Kernel: KernelLinux, ID: "opensuse-leap", IDLike: []string{"suse", "opensuse"}},
			wantKind: Zypper,
		},
		{
			name:     "unknown linux distro",
			info:     OSInfo{Kernel: KernelLinux, ID: "nixos"},
			wantKind: 0,
		},
		{
			name:     "unknown kernel",
			info:     OSInfo{Kernel: "NetBSD"},
			wantKind: 0,
		},
		{
			name:     "empty info",
			info:     OSInfo{},
			wantKind: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := Detect(tt.info)
			if tt.wantKind == 0 {
				if backend != nil {
					t.Errorf("expected nil backend, got %s", backend.Kind)
				}
				return
			}
			if backend == nil {
				t.Fatalf("expected backend %s, got nil", tt.wantKind)
			}
			if backend.Kind != tt.wantKind {
				t.Errorf("backend kind: got %s, want %s", backend.Kind, tt.wantKind)
			}
		})
	}
}

func FuzzParseOSRelease(f *testing.F) {
	f.Add([]byte(`ID=ubuntu
ID_LIKE=debian
`))
	f.Add([]byte(`ID="rocky"
ID_LIKE="rhel centos fedora"
`))
	f.Add([]byte(``))
	f.Add([]byte("# comment\ngarbage\n\x00\xff"))
	f.Add([]byte("ID====weird\nID_LIKE=\"\"\n"))
	f.Add([]byte("=noval\nnoequals\nID\n"))

	f.Fuzz(func(t *testing.T, input []byte) {
		// Must never panic.
		info := ParseOSRelease(input)

		// Kernel is never set by ParseOSRelease.
		if info.Kernel != "" {
			t.Fatalf("Kernel should be empty, got %q", info.Kernel)
		}

		// ID_LIKE entries should never contain whitespace (Fields splits on it).
		for _, like := range info.IDLike {
			for _, r := range like {
				if r == ' ' || r == '\t' || r == '\n' {
					t.Fatalf("IDLike entry %q contains whitespace", like)
				}
			}
		}
	})
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
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
