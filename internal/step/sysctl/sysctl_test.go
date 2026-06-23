// SPDX-License-Identifier: GPL-3.0-only

package sysctl

import "testing"

func TestDropInPath(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{
			"net.ipv4.ip_forward",
			"/etc/sysctl.d/99-scampi-net-ipv4-ip_forward.conf",
		},
		{
			"vm.swappiness",
			"/etc/sysctl.d/99-scampi-vm-swappiness.conf",
		},
		{
			"net.ipv4.conf.all.rp_filter",
			"/etc/sysctl.d/99-scampi-net-ipv4-conf-all-rp_filter.conf",
		},
		{
			"kernel.pid_max",
			"/etc/sysctl.d/99-scampi-kernel-pid_max.conf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := dropInPath(tt.key)
			if got != tt.want {
				t.Errorf("dropInPath(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}
