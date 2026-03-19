// SPDX-License-Identifier: GPL-3.0-only

package pkgmgr

import (
	"testing"

	"scampi.dev/scampi/target"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		platform target.Platform
		wantKind Kind // zero value means nil backend
	}{
		{name: "darwin", platform: target.PlatformDarwin, wantKind: Brew},
		{name: "freebsd", platform: target.PlatformFreeBSD, wantKind: Pkg},
		{name: "debian", platform: target.PlatformDebian, wantKind: Apt},
		{name: "ubuntu", platform: target.PlatformUbuntu, wantKind: Apt},
		{name: "alpine", platform: target.PlatformAlpine, wantKind: Apk},
		{name: "fedora", platform: target.PlatformFedora, wantKind: Dnf},
		{name: "arch", platform: target.PlatformArch, wantKind: Pacman},
		{name: "rhel", platform: target.PlatformRHEL, wantKind: Dnf},
		{name: "suse", platform: target.PlatformSUSE, wantKind: Zypper},
		{name: "unknown", platform: target.PlatformUnknown, wantKind: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := Detect(tt.platform)
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
