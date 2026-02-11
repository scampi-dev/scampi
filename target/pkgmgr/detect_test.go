package pkgmgr

import (
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantID   string
		wantLike []string
	}{
		{
			name: "ubuntu",
			input: `NAME="Ubuntu"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 22.04.3 LTS"
VERSION_ID="22.04"
`,
			wantID:   "ubuntu",
			wantLike: []string{"debian"},
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
		})
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		info     OSInfo
		wantName string // empty string means nil backend
	}{
		{
			name:     "darwin",
			info:     OSInfo{Kernel: KernelDarwin},
			wantName: "brew",
		},
		{
			name:     "freebsd",
			info:     OSInfo{Kernel: KernelFreeBSD},
			wantName: "pkg",
		},
		{
			name:     "debian",
			info:     OSInfo{Kernel: KernelLinux, ID: "debian"},
			wantName: "apt",
		},
		{
			name:     "ubuntu direct",
			info:     OSInfo{Kernel: KernelLinux, ID: "ubuntu", IDLike: []string{"debian"}},
			wantName: "apt",
		},
		{
			name:     "alpine",
			info:     OSInfo{Kernel: KernelLinux, ID: "alpine"},
			wantName: "apk",
		},
		{
			name:     "fedora",
			info:     OSInfo{Kernel: KernelLinux, ID: "fedora"},
			wantName: "dnf",
		},
		{
			name:     "arch",
			info:     OSInfo{Kernel: KernelLinux, ID: "arch"},
			wantName: "pacman",
		},
		{
			name:     "rhel",
			info:     OSInfo{Kernel: KernelLinux, ID: "rhel"},
			wantName: "dnf",
		},
		{
			name:     "suse",
			info:     OSInfo{Kernel: KernelLinux, ID: "suse"},
			wantName: "zypper",
		},
		{
			name:     "ID_LIKE fallback ubuntu to debian",
			info:     OSInfo{Kernel: KernelLinux, ID: "linuxmint", IDLike: []string{"debian", "ubuntu"}},
			wantName: "apt",
		},
		{
			name:     "ID_LIKE fallback rocky to rhel",
			info:     OSInfo{Kernel: KernelLinux, ID: "rocky", IDLike: []string{"rhel", "centos", "fedora"}},
			wantName: "dnf",
		},
		{
			name:     "ID_LIKE fallback opensuse to suse",
			info:     OSInfo{Kernel: KernelLinux, ID: "opensuse-leap", IDLike: []string{"suse", "opensuse"}},
			wantName: "zypper",
		},
		{
			name:     "unknown linux distro",
			info:     OSInfo{Kernel: KernelLinux, ID: "nixos"},
			wantName: "",
		},
		{
			name:     "unknown kernel",
			info:     OSInfo{Kernel: "NetBSD"},
			wantName: "",
		},
		{
			name:     "empty info",
			info:     OSInfo{},
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := Detect(tt.info)
			if tt.wantName == "" {
				if backend != nil {
					t.Errorf("expected nil backend, got %q", backend.Name)
				}
				return
			}
			if backend == nil {
				t.Fatalf("expected backend %q, got nil", tt.wantName)
			}
			if backend.Name != tt.wantName {
				t.Errorf("backend name: got %q, want %q", backend.Name, tt.wantName)
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
