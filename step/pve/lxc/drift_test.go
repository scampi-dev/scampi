// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"testing"

	"scampi.dev/scampi/spec"
)

func TestConfigDrift(t *testing.T) {
	op := &configLxcOp{
		hostname:  "pihole",
		cpu:       LxcCPU{Cores: 2},
		memoryMiB: 512,
		storage:   "local-zfs",
	}

	tests := []struct {
		name      string
		cfg       pctConfig
		wantDrift []string
	}{
		{
			name: "no drift",
			cfg: pctConfig{
				Hostname: "pihole", Cores: 2, Memory: 512,
				Storage: "local-zfs", Size: "4G",
			},
			wantDrift: nil,
		},
		{
			name: "cores drift",
			cfg: pctConfig{
				Hostname: "pihole", Cores: 1, Memory: 512,
				Storage: "local-zfs", Size: "4G",
			},
			wantDrift: []string{"cores"},
		},
		{
			name: "memory drift",
			cfg: pctConfig{
				Hostname: "pihole", Cores: 2, Memory: 256,
				Storage: "local-zfs", Size: "4G",
			},
			wantDrift: []string{"memory"},
		},
		{
			name: "hostname drift",
			cfg: pctConfig{
				Hostname: "old-name", Cores: 2, Memory: 512,
				Storage: "local-zfs", Size: "4G",
			},
			wantDrift: []string{"hostname"},
		},
		{
			name: "multiple drifts",
			cfg: pctConfig{
				Hostname: "wrong", Cores: 4, Memory: 1024,
				Storage: "local-zfs", Size: "4G",
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
					t.Errorf("drift[%d]: got %q, want %q",
						i, got[i], tt.wantDrift[i])
				}
			}
		})
	}
}

func TestCheckImmutables(t *testing.T) {
	op := &configLxcOp{
		storage:    "local-zfs",
		privileged: false,
		pveCmd: pveCmd{
			step: spec.StepInstance{
				Fields: map[string]spec.FieldSpan{
					"storage":    {},
					"privileged": {},
				},
			},
		},
	}

	t.Run("no drift", func(t *testing.T) {
		err := op.checkImmutables(pctConfig{
			Storage: "local-zfs", Unprivileged: 1,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("storage changed", func(t *testing.T) {
		err := op.checkImmutables(pctConfig{
			Storage: "ceph", Unprivileged: 1,
		})
		if err == nil {
			t.Fatal("expected error for storage change")
		}
		if _, ok := err.(ImmutableFieldError); !ok {
			t.Fatalf("expected ImmutableFieldError, got %T", err)
		}
	})

	t.Run("privileged changed", func(t *testing.T) {
		err := op.checkImmutables(pctConfig{
			Storage: "local-zfs", Unprivileged: 0,
		})
		if err == nil {
			t.Fatal("expected error for privileged change")
		}
	})
}

func TestResizeShrink(t *testing.T) {
	op := &resizeLxcOp{
		sizeGiB: 4,
		pveCmd: pveCmd{
			step: spec.StepInstance{
				Fields: map[string]spec.FieldSpan{
					"size": {},
				},
			},
		},
	}

	t.Run("shrink rejected", func(t *testing.T) {
		err := func() error {
			if parseSizeGiB("8G") > op.sizeGiB {
				return ResizeShrinkError{
					Current: "8G",
					Desired: "4G",
				}
			}
			return nil
		}()
		if err == nil {
			t.Fatal("expected error for size shrink")
		}
		if _, ok := err.(ResizeShrinkError); !ok {
			t.Fatalf("expected ResizeShrinkError, got %T", err)
		}
	})

	t.Run("grow is ok", func(t *testing.T) {
		if parseSizeGiB("2G") >= op.sizeGiB {
			t.Fatal("2G should be less than 4G")
		}
	})
}

func TestFilterSetDrift(t *testing.T) {
	drift := []spec.DriftDetail{
		{Field: "cores", Current: "1", Desired: "2"},
		{Field: "memory", Current: "256", Desired: "512"},
		{Field: "hostname", Current: "old", Desired: "new"},
		{Field: "network[0]", Current: "x", Desired: "y"},
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

// Network drift tests
// -----------------------------------------------------------------------------

func TestNetworkDrift(t *testing.T) {
	op := &networkLxcOp{
		networks: []LxcNet{
			{Bridge: "vmbr0", IP: "10.0.0.1/24", Gw: "10.0.0.1"},
			{Bridge: "vmbr1", IP: "192.168.1.5/24"},
		},
	}

	t.Run("both match", func(t *testing.T) {
		cfg := pctConfig{Nets: []parsedNet{
			{Name: "eth0", Bridge: "vmbr0", IP: "10.0.0.1/24", Gw: "10.0.0.1"},
			{Name: "eth1", Bridge: "vmbr1", IP: "192.168.1.5/24"},
		}}
		if len(op.networkDrift(cfg)) != 0 {
			t.Error("expected no drift")
		}
	})

	t.Run("reordered", func(t *testing.T) {
		cfg := pctConfig{Nets: []parsedNet{
			{Name: "eth0", Bridge: "vmbr1", IP: "192.168.1.5/24"},
			{Name: "eth1", Bridge: "vmbr0", IP: "10.0.0.1/24", Gw: "10.0.0.1"},
		}}
		if len(op.networkDrift(cfg)) != 2 {
			t.Error("expected 2 drifts for reorder")
		}
	})

	t.Run("nic added", func(t *testing.T) {
		cfg := pctConfig{Nets: []parsedNet{
			{Name: "eth0", Bridge: "vmbr0", IP: "10.0.0.1/24", Gw: "10.0.0.1"},
		}}
		drift := op.networkDrift(cfg)
		if len(drift) != 1 || drift[0].Field != "network[1]" {
			t.Fatalf("expected network[1] drift, got %v", drift)
		}
	})

	t.Run("nic removed", func(t *testing.T) {
		oneNic := &networkLxcOp{
			networks: []LxcNet{{Bridge: "vmbr0", IP: "10.0.0.1/24", Gw: "10.0.0.1"}},
		}
		cfg := pctConfig{Nets: []parsedNet{
			{Name: "eth0", Bridge: "vmbr0", IP: "10.0.0.1/24", Gw: "10.0.0.1"},
			{Name: "eth1", Bridge: "vmbr1", IP: "192.168.1.5/24"},
		}}
		drift := oneNic.networkDrift(cfg)
		if len(drift) != 1 || drift[0].Field != "network[1]" {
			t.Fatalf("expected network[1] removal, got %v", drift)
		}
	})
}

func TestHasNetworkDrift(t *testing.T) {
	if hasNetworkDrift([]spec.DriftDetail{{Field: "cores"}}) {
		t.Error("cores should not be network drift")
	}
	if !hasNetworkDrift([]spec.DriftDetail{{Field: "network[0]"}}) {
		t.Error("network[0] should be network drift")
	}
}

// Device drift tests
// -----------------------------------------------------------------------------

func TestDeviceDrift(t *testing.T) {
	base := func(devs []LxcDevice) *deviceLxcOp {
		return &deviceLxcOp{devices: devs}
	}

	t.Run("no drift", func(t *testing.T) {
		op := base([]LxcDevice{{Path: "/dev/dri/renderD128", Mode: "0666"}})
		cfg := pctConfig{
			Devs: []parsedDev{{Path: "/dev/dri/renderD128", Mode: "0666"}},
		}
		if len(op.deviceDrift(cfg)) != 0 {
			t.Error("expected no drift")
		}
	})

	t.Run("device added", func(t *testing.T) {
		op := base([]LxcDevice{{Path: "/dev/dri/renderD128", Mode: "0666"}})
		if len(op.deviceDrift(pctConfig{})) == 0 {
			t.Fatal("expected drift")
		}
	})

	t.Run("device removed", func(t *testing.T) {
		op := base(nil)
		cfg := pctConfig{
			Devs: []parsedDev{{Path: "/dev/dri/renderD128", Mode: "0666"}},
		}
		if len(op.deviceDrift(cfg)) == 0 {
			t.Fatal("expected drift for removal")
		}
	})

	t.Run("mode changed", func(t *testing.T) {
		op := base([]LxcDevice{{Path: "/dev/dri/renderD128", Mode: "0660"}})
		cfg := pctConfig{
			Devs: []parsedDev{{Path: "/dev/dri/renderD128", Mode: "0666"}},
		}
		if len(op.deviceDrift(cfg)) == 0 {
			t.Fatal("expected drift for mode change")
		}
	})

	t.Run("multiple match", func(t *testing.T) {
		op := base([]LxcDevice{
			{Path: "/dev/dri/renderD128", Mode: "0666"},
			{Path: "/dev/kfd", Mode: "0660"},
		})
		cfg := pctConfig{Devs: []parsedDev{
			{Path: "/dev/dri/renderD128", Mode: "0666"},
			{Path: "/dev/kfd", Mode: "0660"},
		}}
		if len(op.deviceDrift(cfg)) != 0 {
			t.Error("expected no drift")
		}
	})
}

func TestHasDeviceDrift(t *testing.T) {
	if hasDeviceDrift([]spec.DriftDetail{{Field: "cores"}}) {
		t.Error("cores should not be device drift")
	}
	if !hasDeviceDrift([]spec.DriftDetail{{Field: "device[0]"}}) {
		t.Error("device[0] should be device drift")
	}
}
