// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"errors"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/spec"
)

// Parsing
// -----------------------------------------------------------------------------

func TestParseMpKey(t *testing.T) {
	cases := []struct {
		key string
		idx int
		ok  bool
	}{
		{"mp0", 0, true},
		{"mp1", 1, true},
		{"mp12", 12, true},
		{"net0", 0, false},
		{"mpX", 0, false},
		{"mp", 0, false},
	}
	for _, tt := range cases {
		idx, ok := parseMpKey(tt.key)
		if ok != tt.ok || (ok && idx != tt.idx) {
			t.Errorf("parseMpKey(%q) = (%d, %v), want (%d, %v)",
				tt.key, idx, ok, tt.idx, tt.ok)
		}
	}
}

func TestParseMpValue_BindMount(t *testing.T) {
	m := parseMpValue("/mnt/data,mp=/data")
	if m.Kind != MountBind {
		t.Errorf("kind = %v, want MountBind", m.Kind)
	}
	if m.Source != "/mnt/data" {
		t.Errorf("source = %q, want %q", m.Source, "/mnt/data")
	}
	if m.Mountpoint != "/data" {
		t.Errorf("mountpoint = %q, want %q", m.Mountpoint, "/data")
	}
	if m.ReadOnly {
		t.Error("should not be read-only")
	}
	if !m.Backup {
		t.Error("backup should default to true")
	}
}

func TestParseMpValue_BindMountWithOptions(t *testing.T) {
	m := parseMpValue("/mnt/media,mp=/media,ro=1,backup=0")
	if m.Kind != MountBind {
		t.Fatalf("kind = %v, want MountBind", m.Kind)
	}
	if !m.ReadOnly {
		t.Error("should be read-only")
	}
	if m.Backup {
		t.Error("backup should be false")
	}
}

func TestParseMpValue_VolumeMount(t *testing.T) {
	m := parseMpValue("local-zfs:subvol-100-disk-1,mp=/backup,size=50G")
	if m.Kind != MountVolume {
		t.Errorf("kind = %v, want MountVolume", m.Kind)
	}
	if m.Storage != "local-zfs" {
		t.Errorf("storage = %q, want %q", m.Storage, "local-zfs")
	}
	if m.Mountpoint != "/backup" {
		t.Errorf("mountpoint = %q, want %q", m.Mountpoint, "/backup")
	}
	if m.Size != "50G" {
		t.Errorf("size = %q, want %q", m.Size, "50G")
	}
}

func TestParseMpValue_VolumeMountWithOptions(t *testing.T) {
	m := parseMpValue("local-lvm:vm-200-disk-1,mp=/data,size=100G,ro=1,backup=0")
	if m.Kind != MountVolume {
		t.Fatalf("kind = %v, want MountVolume", m.Kind)
	}
	if m.Storage != "local-lvm" {
		t.Errorf("storage = %q", m.Storage)
	}
	if m.Size != "100G" {
		t.Errorf("size = %q", m.Size)
	}
	if !m.ReadOnly {
		t.Error("should be read-only")
	}
	if m.Backup {
		t.Error("backup should be false")
	}
}

// Formatting
// -----------------------------------------------------------------------------

func TestFormatMp_BindMount(t *testing.T) {
	m := LxcMount{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data", Backup: true}
	got := formatMp(m)
	want := "/mnt/data,mp=/data"
	if got != want {
		t.Errorf("formatMp = %q, want %q", got, want)
	}
}

func TestFormatMp_BindMountWithOptions(t *testing.T) {
	m := LxcMount{Kind: MountBind, Source: "/mnt/media", Mountpoint: "/media", ReadOnly: true, Backup: false}
	got := formatMp(m)
	want := "/mnt/media,mp=/media,ro=1,backup=0"
	if got != want {
		t.Errorf("formatMp = %q, want %q", got, want)
	}
}

func TestFormatMp_VolumeMount(t *testing.T) {
	m := LxcMount{Kind: MountVolume, Storage: "local-zfs", Mountpoint: "/backup", Size: "50G", Backup: true}
	got := formatMp(m)
	want := "local-zfs:50,mp=/backup,size=50G"
	if got != want {
		t.Errorf("formatMp = %q, want %q", got, want)
	}
}

// Roundtrip
// -----------------------------------------------------------------------------

func TestMountRoundtrip_Bind(t *testing.T) {
	val := "/mnt/data,mp=/data"
	parsed := parseMpValue(val)
	m := parsedToLxcMount(parsed)
	got := formatMp(m)
	if got != val {
		t.Errorf("roundtrip %q → %+v → %q", val, m, got)
	}
}

func TestMountRoundtrip_BindWithOptions(t *testing.T) {
	val := "/mnt/media,mp=/media,ro=1,backup=0"
	parsed := parseMpValue(val)
	m := parsedToLxcMount(parsed)
	got := formatMp(m)
	if got != val {
		t.Errorf("roundtrip %q → %+v → %q", val, m, got)
	}
}

// Config parsing
// -----------------------------------------------------------------------------

func TestParsePctConfigMounts(t *testing.T) {
	output := "arch: amd64\n" +
		"cores: 2\n" +
		"mp0: /mnt/data,mp=/data\n" +
		"mp1: local-zfs:subvol-100-disk-1,mp=/backup,size=50G\n" +
		"hostname: nas\n"
	cfg := parsePctConfig(output)
	if len(cfg.Mounts) != 2 {
		t.Fatalf("got %d mounts, want 2", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Kind != MountBind {
		t.Errorf("mp0 kind = %v, want MountBind", cfg.Mounts[0].Kind)
	}
	if cfg.Mounts[0].Source != "/mnt/data" {
		t.Errorf("mp0 source = %q", cfg.Mounts[0].Source)
	}
	if cfg.Mounts[1].Kind != MountVolume {
		t.Errorf("mp1 kind = %v, want MountVolume", cfg.Mounts[1].Kind)
	}
	if cfg.Mounts[1].Storage != "local-zfs" {
		t.Errorf("mp1 storage = %q", cfg.Mounts[1].Storage)
	}
}

// Drift
// -----------------------------------------------------------------------------

func TestMountDrift(t *testing.T) {
	base := func(mounts []LxcMount) *mountLxcOp {
		return &mountLxcOp{mounts: mounts}
	}

	t.Run("no drift", func(t *testing.T) {
		op := base([]LxcMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data", Backup: true},
		})
		cfg := pctConfig{Mounts: []parsedMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data", Backup: true},
		}}
		if len(op.mountDrift(cfg)) != 0 {
			t.Error("expected no drift")
		}
	})

	t.Run("mount added", func(t *testing.T) {
		op := base([]LxcMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data", Backup: true},
		})
		cfg := pctConfig{}
		drift := op.mountDrift(cfg)
		if len(drift) == 0 {
			t.Fatal("expected drift for add")
		}
		if drift[0].Current != "(absent)" {
			t.Errorf("current = %q", drift[0].Current)
		}
	})

	t.Run("mount removed", func(t *testing.T) {
		op := base(nil)
		cfg := pctConfig{Mounts: []parsedMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data", Backup: true},
		}}
		drift := op.mountDrift(cfg)
		if len(drift) == 0 {
			t.Fatal("expected drift for removal")
		}
		if drift[0].Desired != "(absent)" {
			t.Errorf("desired = %q", drift[0].Desired)
		}
	})

	t.Run("mountpoint changed", func(t *testing.T) {
		op := base([]LxcMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/new-data", Backup: true},
		})
		cfg := pctConfig{Mounts: []parsedMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data", Backup: true},
		}}
		if len(op.mountDrift(cfg)) == 0 {
			t.Error("expected drift for mountpoint change")
		}
	})

	t.Run("options changed", func(t *testing.T) {
		op := base([]LxcMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data", ReadOnly: true, Backup: true},
		})
		cfg := pctConfig{Mounts: []parsedMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data", ReadOnly: false, Backup: true},
		}}
		if len(op.mountDrift(cfg)) == 0 {
			t.Error("expected drift for ro change")
		}
	})
}

// Create command
// -----------------------------------------------------------------------------

func TestBuildCreateCmdMounts(t *testing.T) {
	act := lxcAction{
		id:        100,
		template:  &LxcTemplate{Storage: "local", Name: "debian.tar.zst"},
		hostname:  "test",
		state:     StateRunning,
		cpu:       LxcCPU{Cores: 1},
		memoryMiB: 512,
		swapMiB:   512,
		storage:   "local-zfs",
		sizeGiB:   8,
		mounts: []LxcMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data", Backup: true},
			{Kind: MountVolume, Storage: "local-zfs", Mountpoint: "/backup", Size: "50G", Backup: true},
		},
	}
	cmd := buildCreateCmd(act)
	if !contains(cmd, "--mp0 /mnt/data,mp=/data") {
		t.Errorf("cmd missing bind mount: %s", cmd)
	}
	if !contains(cmd, "--mp1 local-zfs:50,mp=/backup,size=50G") {
		t.Errorf("cmd missing volume mount: %s", cmd)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Validate
// -----------------------------------------------------------------------------

func TestValidateMounts(t *testing.T) {
	step := spec.StepInstance{
		Fields: map[string]spec.FieldSpan{"mounts": {}},
	}

	t.Run("valid bind", func(t *testing.T) {
		cfg := &LxcConfig{
			ID: 100, Node: "n", Hostname: "h", State: "running",
			Template: &LxcTemplate{Name: "t"},
			Memory:   "512M", Size: "8G",
			Mounts: []LxcMount{
				{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data"},
			},
		}
		if err := cfg.validate(step); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing mountpoint", func(t *testing.T) {
		cfg := &LxcConfig{
			ID: 100, Node: "n", Hostname: "h", State: "running",
			Template: &LxcTemplate{Name: "t"},
			Memory:   "512M", Size: "8G",
			Mounts: []LxcMount{
				{Kind: MountBind, Source: "/mnt/data"},
			},
		}
		if err := cfg.validate(step); err == nil {
			t.Error("expected error for missing mountpoint")
		}
	})

	t.Run("duplicate mountpoint", func(t *testing.T) {
		cfg := &LxcConfig{
			ID: 100, Node: "n", Hostname: "h", State: "running",
			Template: &LxcTemplate{Name: "t"},
			Memory:   "512M", Size: "8G",
			Mounts: []LxcMount{
				{Kind: MountBind, Source: "/a", Mountpoint: "/data"},
				{Kind: MountBind, Source: "/b", Mountpoint: "/data"},
			},
		}
		if err := cfg.validate(step); err == nil {
			t.Error("expected error for duplicate mountpoint")
		}
	})

	t.Run("bind missing source", func(t *testing.T) {
		cfg := &LxcConfig{
			ID: 100, Node: "n", Hostname: "h", State: "running",
			Template: &LxcTemplate{Name: "t"},
			Memory:   "512M", Size: "8G",
			Mounts: []LxcMount{
				{Kind: MountBind, Mountpoint: "/data"},
			},
		}
		if err := cfg.validate(step); err == nil {
			t.Error("expected error for bind mount without source")
		}
	})

	t.Run("volume missing size", func(t *testing.T) {
		cfg := &LxcConfig{
			ID: 100, Node: "n", Hostname: "h", State: "running",
			Template: &LxcTemplate{Name: "t"},
			Memory:   "512M", Size: "8G",
			Mounts: []LxcMount{
				{Kind: MountVolume, Storage: "local-zfs", Mountpoint: "/data"},
			},
		}
		if err := cfg.validate(step); err == nil {
			t.Error("expected error for volume mount without size")
		}
	})
}

// Inputs / DeferredResource
// -----------------------------------------------------------------------------

func TestLxcAction_Inputs_BindMountSources(t *testing.T) {
	act := &lxcAction{
		mounts: []LxcMount{
			{Kind: MountBind, Source: "/mnt/data", Mountpoint: "/data"},
			{Kind: MountVolume, Storage: "local-zfs", Mountpoint: "/vol", Size: "50G"},
			{Kind: MountBind, Source: "/mnt/media", Mountpoint: "/media"},
		},
	}
	inputs := act.Inputs()
	if len(inputs) != 2 {
		t.Fatalf("got %d inputs, want 2 (only bind mounts)", len(inputs))
	}
	if inputs[0] != spec.PathResource("/mnt/data") {
		t.Errorf("input[0] = %v, want PathResource(/mnt/data)", inputs[0])
	}
	if inputs[1] != spec.PathResource("/mnt/media") {
		t.Errorf("input[1] = %v, want PathResource(/mnt/media)", inputs[1])
	}
}

func TestLxcAction_Inputs_NoMounts(t *testing.T) {
	act := &lxcAction{}
	if inputs := act.Inputs(); len(inputs) != 0 {
		t.Errorf("got %d inputs, want 0", len(inputs))
	}
}

// Promises
// -----------------------------------------------------------------------------

func TestLxcAction_Promises_PerVMID(t *testing.T) {
	cases := []struct {
		node string
		id   int
		want string
	}{
		{"midgard", 100, "pve://midgard/100"},
		{"midgard", 101, "pve://midgard/101"},
		{"asgard", 100, "pve://asgard/100"},
	}
	for _, tc := range cases {
		act := &lxcAction{node: tc.node, id: tc.id}
		got := act.Promises()
		if len(got) != 1 {
			t.Fatalf("node=%s id=%d: got %d promises, want 1", tc.node, tc.id, len(got))
		}
		if got[0] != spec.ContainerResource(tc.want) {
			t.Errorf("node=%s id=%d: got %v, want ContainerResource(%q)", tc.node, tc.id, got[0], tc.want)
		}
	}
}

func TestLxcAction_Promises_DistinctVMIDsAreIndependent(t *testing.T) {
	a := &lxcAction{node: "midgard", id: 100}
	b := &lxcAction{node: "midgard", id: 101}
	if a.Promises()[0] == b.Promises()[0] {
		t.Error("distinct VMIDs should produce distinct resource keys")
	}
}

func TestBindSourceMissingError_IsDeferrable(t *testing.T) {
	err := BindSourceMissingError{Path: "/mnt/data"}

	var d diagnostic.Deferrable
	if !errors.As(err, &d) {
		t.Fatal("BindSourceMissingError must implement diagnostic.Deferrable")
	}

	res := d.DeferredResource()
	want := spec.PathResource("/mnt/data")
	if res != want {
		t.Errorf("DeferredResource() = %v, want %v", res, want)
	}
}

func TestBindSourceMissingError_Message(t *testing.T) {
	err := BindSourceMissingError{Path: "/mnt/data"}
	msg := err.Error()
	if !contains(msg, "/mnt/data") {
		t.Errorf("error message should mention the path, got %q", msg)
	}
}
