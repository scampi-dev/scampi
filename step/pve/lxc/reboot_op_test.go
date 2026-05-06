// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"strings"
	"testing"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

type mockTarget struct {
	handler func(cmd string) (target.CommandResult, error)
}

func (m *mockTarget) Capabilities() capability.Capability {
	return capability.PVE | capability.Command
}

func (m *mockTarget) RunCommand(_ context.Context, cmd string) (target.CommandResult, error) {
	return m.handler(cmd)
}

func (m *mockTarget) RunPrivileged(_ context.Context, cmd string) (target.CommandResult, error) {
	return m.handler(cmd)
}

func TestRebootOp_RunsChecksAndDetectsDrift(t *testing.T) {
	cmdr := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
		if cmd == "pct status 100" {
			return target.CommandResult{Stdout: "status: running\n"}, nil
		}
		return target.CommandResult{ExitCode: 1}, nil
	}}

	op := &rebootLxcOp{
		pveCmd: pveCmd{id: 100},
		checks: []rebootCheck{
			{field: "test", desired: "wanted", probe: func(_ context.Context, _ target.Command, _ int) string {
				return "actual"
			}},
		},
	}
	result, drift, err := op.checkWith(context.Background(), cmdr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != spec.CheckUnsatisfied {
		t.Errorf("got %v, want CheckUnsatisfied", result)
	}
	if len(drift) != 1 || drift[0].Field != "test (reboot)" {
		t.Errorf("got drift %v, want test (reboot)", drift)
	}
}

func TestRebootOp_NoDriftWhenMatched(t *testing.T) {
	cmdr := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
		if cmd == "pct status 100" {
			return target.CommandResult{Stdout: "status: running\n"}, nil
		}
		return target.CommandResult{ExitCode: 1}, nil
	}}

	op := &rebootLxcOp{
		pveCmd: pveCmd{id: 100},
		checks: []rebootCheck{
			{field: "test", desired: "same", probe: func(_ context.Context, _ target.Command, _ int) string {
				return "same"
			}},
		},
	}
	result, _, err := op.checkWith(context.Background(), cmdr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != spec.CheckSatisfied {
		t.Errorf("got %v, want CheckSatisfied", result)
	}
}

func TestRebootOp_SkippedWhenStopped(t *testing.T) {
	cmdr := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
		if cmd == "pct status 100" {
			return target.CommandResult{Stdout: "status: stopped\n"}, nil
		}
		return target.CommandResult{ExitCode: 1}, nil
	}}

	op := &rebootLxcOp{
		pveCmd: pveCmd{id: 100},
		checks: []rebootCheck{
			{field: "test", desired: "wanted", probe: func(_ context.Context, _ target.Command, _ int) string {
				return "different"
			}},
		},
	}
	result, _, err := op.checkWith(context.Background(), cmdr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != spec.CheckSatisfied {
		t.Error("got unsatisfied, want CheckSatisfied (no reboot for stopped)")
	}
}

// Op-level reboot check tests
// -----------------------------------------------------------------------------

func TestConfigOp_RebootChecks_Hostname(t *testing.T) {
	op := &configLxcOp{hostname: "pihole"}
	checks := op.RebootChecks()

	var found bool
	for _, c := range checks {
		if c.field == "hostname" {
			found = true
			if c.desired != "pihole" {
				t.Errorf("desired = %q, want pihole", c.desired)
			}
		}
	}
	if !found {
		t.Error("no hostname reboot check")
	}
}

func TestConfigOp_RebootChecks_Features(t *testing.T) {
	op := &configLxcOp{
		features: &LxcFeatures{Nesting: true, Keyctl: true},
	}
	checks := op.RebootChecks()

	var found bool
	for _, c := range checks {
		if c.field == "features" {
			found = true
			if c.desired != "nesting=1,keyctl=1" {
				t.Errorf("desired = %q", c.desired)
			}
		}
	}
	if !found {
		t.Error("no features reboot check")
	}
}

func TestConfigOp_RebootChecks_NoFeaturesWhenNil(t *testing.T) {
	op := &configLxcOp{hostname: "test"}
	for _, c := range op.RebootChecks() {
		if c.field == "features" {
			t.Error("should not have features check when nil")
		}
	}
}

func TestConfigOp_RebootChecks_DNS(t *testing.T) {
	op := &configLxcOp{
		dns: LxcDNS{Nameserver: []string{"1.1.1.1"}, Searchdomain: "local"},
	}
	checks := op.RebootChecks()

	var found bool
	for _, c := range checks {
		if c.field == "dns" {
			found = true
			if c.desired != "1.1.1.1|local" {
				t.Errorf("dns desired = %q, want 1.1.1.1|local", c.desired)
			}
		}
	}
	if !found {
		t.Error("no dns check")
	}
}

// TestConfigOp_RebootChecks_DNS_ProbesResolvConf is the design check
// for #242: the DNS reboot probe must read the running container's
// /etc/resolv.conf, not `pct config`. Otherwise `pct set --nameserver=""`
// would update the config file (which the probe would then read as
// matching desired), and the reboot op would skip — leaving the
// container with stale resolv.conf until next manual reboot.
func TestConfigOp_RebootChecks_DNS_ProbesResolvConf(t *testing.T) {
	var probedCmds []string
	cmdr := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
		probedCmds = append(probedCmds, cmd)
		if cmd == "pct exec 100 -- cat /etc/resolv.conf" {
			return target.CommandResult{
				Stdout: "search lan\nnameserver 1.1.1.1\n",
			}, nil
		}
		return target.CommandResult{ExitCode: 1}, nil
	}}

	op := &configLxcOp{
		pveCmd: pveCmd{id: 100},
		dns:    LxcDNS{Nameserver: []string{"8.8.8.8"}, Searchdomain: "wan"},
	}

	var dnsCheck *rebootCheck
	for i, c := range op.RebootChecks() {
		if c.field == "dns" {
			dnsCheck = &op.RebootChecks()[i]
			break
		}
	}
	if dnsCheck == nil {
		t.Fatal("no dns reboot check")
	}

	current := dnsCheck.probe(context.Background(), cmdr, 100)

	if current != "1.1.1.1|lan" {
		t.Errorf("probed = %q, want 1.1.1.1|lan", current)
	}
	if dnsCheck.desired != "8.8.8.8|wan" {
		t.Errorf("desired = %q, want 8.8.8.8|wan", dnsCheck.desired)
	}
	// Critical: probe must not have touched `pct config` — that path is
	// the one that misses runtime drift after a `pct set --nameserver=""`.
	for _, cmd := range probedCmds {
		if cmd == "pct config 100" {
			t.Errorf("DNS probe queried pct config — should probe runtime resolv.conf instead")
		}
	}
}

// TestConfigOp_Execute_NoReboot is the regression for #242: configLxcOp
// must not issue `pct reboot` from Execute. Reboot is rebootLxcOp's
// responsibility, wired via RebootChecks().
func TestConfigOp_Execute_NoReboot(t *testing.T) {
	var commands []string
	cmdr := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
		commands = append(commands, cmd)
		switch {
		case cmd == "pct list":
			return target.CommandResult{
				Stdout: "VMID Status Lock Name\n100 running    test\n",
			}, nil
		case cmd == "pct status 100":
			return target.CommandResult{Stdout: "status: running\n"}, nil
		case cmd == "pct config 100":
			// hostname matches; only DNS drifts.
			return target.CommandResult{
				Stdout: "hostname: test\nnameserver: 1.1.1.1\n",
			}, nil
		case strings.HasPrefix(cmd, "pct set "):
			return target.CommandResult{}, nil
		}
		return target.CommandResult{ExitCode: 1}, nil
	}}

	op := &configLxcOp{
		pveCmd: pveCmd{
			id:   100,
			step: spec.StepInstance{Fields: map[string]spec.FieldSpan{}},
		},
		hostname:  "test",
		cpu:       LxcCPU{Cores: 1},
		memoryMiB: 512,
		storage:   "local-zfs",
		dns:       LxcDNS{Nameserver: []string{"8.8.8.8"}},
	}

	if _, err := op.Execute(context.Background(), nil, cmdr); err != nil {
		t.Logf("Execute error (expected for partial mock): %v", err)
	}

	for _, cmd := range commands {
		if strings.Contains(cmd, "pct reboot") {
			t.Errorf("configLxcOp.Execute issued reboot — should be left to rebootLxcOp: %q", cmd)
		}
	}
}

func TestDeviceOp_RebootChecks(t *testing.T) {
	op := &deviceLxcOp{
		devices: []LxcDevice{{Path: "/dev/dri/renderD128", Mode: "0666"}},
	}
	checks := op.RebootChecks()
	if len(checks) != 1 {
		t.Fatalf("got %d checks, want 1", len(checks))
	}
	if checks[0].field != "devices" {
		t.Errorf("field = %q, want devices", checks[0].field)
	}
}

func TestNetworkOp_NoRebootChecks(t *testing.T) {
	op := &networkLxcOp{
		networks: []LxcNet{{Bridge: "vmbr0", IP: "10.0.0.1/24"}},
	}
	if _, ok := any(op).(rebootAware); ok {
		t.Error("networkOp should not be rebootAware")
	}
}
