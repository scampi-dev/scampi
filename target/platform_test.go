// SPDX-License-Identifier: GPL-3.0-only

package target

import (
	"testing"
)

func TestResolveLinuxPlatform(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantPlatform Platform
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
			wantPlatform: PlatformUbuntu,
			wantCodename: "jammy",
		},
		{
			name: "alpine",
			input: `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.19.0
PRETTY_NAME="Alpine Linux v3.19"
`,
			wantPlatform: PlatformAlpine,
		},
		{
			name: "rocky linux (falls back to rhel via ID_LIKE)",
			input: `NAME="Rocky Linux"
VERSION="9.3 (Blue Onyx)"
ID="rocky"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.3"
PRETTY_NAME="Rocky Linux 9.3 (Blue Onyx)"
`,
			wantPlatform: PlatformRHEL,
		},
		{
			name: "fedora",
			input: `NAME="Fedora Linux"
VERSION="39 (Workstation Edition)"
ID=fedora
VERSION_ID=39
PRETTY_NAME="Fedora Linux 39 (Workstation Edition)"
`,
			wantPlatform: PlatformFedora,
		},
		{
			name: "arch",
			input: `NAME="Arch Linux"
PRETTY_NAME="Arch Linux"
ID=arch
BUILD_ID=rolling
`,
			wantPlatform: PlatformArch,
		},
		{
			name: "opensuse (falls back to suse via ID_LIKE)",
			input: `NAME="openSUSE Leap"
VERSION="15.5"
ID="opensuse-leap"
ID_LIKE="suse opensuse"
PRETTY_NAME="openSUSE Leap 15.5"
`,
			wantPlatform: PlatformSUSE,
		},
		{
			name:         "empty",
			input:        "",
			wantPlatform: PlatformUnknown,
		},
		{
			name:         "comments only",
			input:        "# this is a comment\n# another comment\n",
			wantPlatform: PlatformUnknown,
		},
		{
			name: "debian with codename",
			input: `ID=debian
ID_LIKE=""
VERSION_CODENAME=bookworm
VERSION_ID="12"
`,
			wantPlatform: PlatformDebian,
			wantCodename: "bookworm",
		},
		{
			name: "linuxmint (falls back to debian via ID_LIKE)",
			input: `ID=linuxmint
ID_LIKE="debian ubuntu"
`,
			wantPlatform: PlatformDebian,
		},
		{
			name:         "nixos (unknown, no ID_LIKE fallback)",
			input:        "ID=nixos\n",
			wantPlatform: PlatformUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ResolveLinuxPlatform([]byte(tt.input))
			if info.Platform != tt.wantPlatform {
				t.Errorf("Platform: got %v, want %v", info.Platform, tt.wantPlatform)
			}
			if info.VersionCodename != tt.wantCodename {
				t.Errorf("VersionCodename: got %q, want %q", info.VersionCodename, tt.wantCodename)
			}
		})
	}
}

func TestParseKernel(t *testing.T) {
	tests := []struct {
		input string
		want  Platform
	}{
		{"Linux", PlatformUnknown},
		{"Darwin", PlatformDarwin},
		{"FreeBSD", PlatformFreeBSD},
		{"NetBSD", PlatformUnknown},
		{"", PlatformUnknown},
	}

	for _, tt := range tests {
		if got := ParseKernel(tt.input); got != tt.want {
			t.Errorf("ParseKernel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPlatformIsGNU(t *testing.T) {
	gnu := []Platform{
		PlatformDebian, PlatformUbuntu, PlatformAlpine,
		PlatformFedora, PlatformRHEL, PlatformArch, PlatformSUSE,
	}
	for _, p := range gnu {
		if !p.IsGNU() {
			t.Errorf("%v.IsGNU() = false, want true", p)
		}
	}

	notGNU := []Platform{PlatformUnknown, PlatformDarwin, PlatformFreeBSD}
	for _, p := range notGNU {
		if p.IsGNU() {
			t.Errorf("%v.IsGNU() = true, want false", p)
		}
	}
}

func TestPlatformIsBSD(t *testing.T) {
	bsd := []Platform{PlatformDarwin, PlatformFreeBSD}
	for _, p := range bsd {
		if !p.IsBSD() {
			t.Errorf("%v.IsBSD() = false, want true", p)
		}
	}

	notBSD := []Platform{
		PlatformUnknown, PlatformDebian, PlatformUbuntu, PlatformAlpine,
		PlatformFedora, PlatformRHEL, PlatformArch, PlatformSUSE,
	}
	for _, p := range notBSD {
		if p.IsBSD() {
			t.Errorf("%v.IsBSD() = true, want false", p)
		}
	}
}

func FuzzResolveLinuxPlatform(f *testing.F) {
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

	f.Fuzz(func(_ *testing.T, input []byte) {
		// Must never panic.
		ResolveLinuxPlatform(input)
	})
}
