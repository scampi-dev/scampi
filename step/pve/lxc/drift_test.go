// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"testing"

	"scampi.dev/scampi/spec"
)

func TestConfigDrift(t *testing.T) {
	op := &ensureLxcOp{
		hostname:  "pihole",
		cores:     2,
		memoryMiB: 512,
		storage:   "local-zfs",
		sizeGiB:   4,
		network:   LxcNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
	}

	tests := []struct {
		name      string
		cfg       pctConfig
		wantDrift []string // expected drift field names
	}{
		{
			name: "no drift",
			cfg: pctConfig{
				Hostname: "pihole",
				Cores:    2,
				Memory:   512,
				Storage:  "local-zfs",
				Size:     "4G",
				Net:      parsedNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
			},
			wantDrift: nil,
		},
		{
			name: "cores drift",
			cfg: pctConfig{
				Hostname: "pihole",
				Cores:    1,
				Memory:   512,
				Storage:  "local-zfs",
				Size:     "4G",
				Net:      parsedNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
			},
			wantDrift: []string{"cores"},
		},
		{
			name: "memory drift",
			cfg: pctConfig{
				Hostname: "pihole",
				Cores:    2,
				Memory:   256,
				Storage:  "local-zfs",
				Size:     "4G",
				Net:      parsedNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
			},
			wantDrift: []string{"memory"},
		},
		{
			name: "hostname drift",
			cfg: pctConfig{
				Hostname: "old-name",
				Cores:    2,
				Memory:   512,
				Storage:  "local-zfs",
				Size:     "4G",
				Net:      parsedNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
			},
			wantDrift: []string{"hostname"},
		},
		{
			name: "network ip drift",
			cfg: pctConfig{
				Hostname: "pihole",
				Cores:    2,
				Memory:   512,
				Storage:  "local-zfs",
				Size:     "4G",
				Net:      parsedNet{Bridge: "vmbr0", IP: "192.168.1.5/24", Gw: "10.10.10.1"},
			},
			wantDrift: []string{"network.ip"},
		},
		{
			name: "network gw added",
			cfg: pctConfig{
				Hostname: "pihole",
				Cores:    2,
				Memory:   512,
				Storage:  "local-zfs",
				Size:     "4G",
				Net:      parsedNet{Bridge: "vmbr0", IP: "10.10.10.10/24"},
			},
			wantDrift: []string{"network.gw"},
		},
		{
			name: "size grow",
			cfg: pctConfig{
				Hostname: "pihole",
				Cores:    2,
				Memory:   512,
				Storage:  "local-zfs",
				Size:     "2G",
				Net:      parsedNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
			},
			wantDrift: []string{"size"},
		},
		{
			name: "multiple drifts",
			cfg: pctConfig{
				Hostname: "wrong",
				Cores:    4,
				Memory:   1024,
				Storage:  "local-zfs",
				Size:     "4G",
				Net:      parsedNet{Bridge: "vmbr0", IP: "10.10.10.10/24", Gw: "10.10.10.1"},
			},
			wantDrift: []string{"cores", "memory", "hostname"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drift := op.configDrift(tt.cfg)
			got := make([]string, len(drift))
			for i, d := range drift {
				got[i] = d.Field
			}
			if len(got) != len(tt.wantDrift) {
				t.Fatalf("got %v, want %v", got, tt.wantDrift)
			}
			for i := range got {
				if got[i] != tt.wantDrift[i] {
					t.Errorf("drift[%d]: got %q, want %q", i, got[i], tt.wantDrift[i])
				}
			}
		})
	}
}

func TestCheckImmutables(t *testing.T) {
	op := &ensureLxcOp{
		storage: "local-zfs",
		sizeGiB: 4,
		step: spec.StepInstance{
			Fields: map[string]spec.FieldSpan{
				"storage": {},
				"size":    {},
			},
		},
	}

	t.Run("no drift", func(t *testing.T) {
		err := op.checkImmutables(pctConfig{Storage: "local-zfs", Size: "4G"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("storage changed", func(t *testing.T) {
		err := op.checkImmutables(pctConfig{Storage: "ceph", Size: "4G"})
		if err == nil {
			t.Fatal("expected error for storage change")
		}
		if _, ok := err.(ImmutableFieldError); !ok {
			t.Fatalf("expected ImmutableFieldError, got %T", err)
		}
	})

	t.Run("size shrink", func(t *testing.T) {
		err := op.checkImmutables(pctConfig{Storage: "local-zfs", Size: "8G"})
		if err == nil {
			t.Fatal("expected error for size shrink")
		}
		if _, ok := err.(ResizeShrinkError); !ok {
			t.Fatalf("expected ResizeShrinkError, got %T", err)
		}
	})

	t.Run("size grow is ok", func(t *testing.T) {
		err := op.checkImmutables(pctConfig{Storage: "local-zfs", Size: "2G"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestFilterSetDrift(t *testing.T) {
	drift := []spec.DriftDetail{
		{Field: "cores", Current: "1", Desired: "2"},
		{Field: "memory", Current: "256", Desired: "512"},
		{Field: "hostname", Current: "old", Desired: "new"},
		{Field: "network.ip", Current: "1.2.3.4/24", Desired: "5.6.7.8/24"},
		{Field: "size", Current: "4G", Desired: "8G"},
	}

	set := filterSetDrift(drift)
	if len(set) != 3 {
		t.Fatalf("got %d set drifts, want 3", len(set))
	}
	for _, d := range set {
		switch d.Field {
		case "cores", "memory", "hostname":
		default:
			t.Errorf("unexpected field in set drift: %q", d.Field)
		}
	}
}

func TestHasNetworkDrift(t *testing.T) {
	if hasNetworkDrift([]spec.DriftDetail{{Field: "cores"}}) {
		t.Error("cores should not be network drift")
	}
	if !hasNetworkDrift([]spec.DriftDetail{{Field: "network.ip"}}) {
		t.Error("network.ip should be network drift")
	}
	if !hasNetworkDrift([]spec.DriftDetail{{Field: "network.gw"}}) {
		t.Error("network.gw should be network drift")
	}
}
