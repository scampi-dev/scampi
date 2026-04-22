// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"context"
	"testing"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// mockTarget implements target.Target + target.Command for testing ops.
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

func TestRebootOpCheck_NoRebootNeeded(t *testing.T) {
	cmdr := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
		switch cmd {
		case "pct status 100":
			return target.CommandResult{Stdout: "status: running\n"}, nil
		case "pct exec 100 -- hostname":
			return target.CommandResult{Stdout: "pihole\n"}, nil
		default:
			return target.CommandResult{ExitCode: 1}, nil
		}
	}}

	op := &rebootLxcOp{pveCmd: pveCmd{id: 100}, hostname: "pihole"}
	result, drift, err := op.checkWith(context.Background(), cmdr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != spec.CheckSatisfied {
		t.Errorf("got %v, want CheckSatisfied", result)
	}
	if len(drift) != 0 {
		t.Errorf("got %d drift entries, want 0", len(drift))
	}
}

func TestRebootOpCheck_HostnameMismatch(t *testing.T) {
	cmdr := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
		switch cmd {
		case "pct status 100":
			return target.CommandResult{Stdout: "status: running\n"}, nil
		case "pct exec 100 -- hostname":
			return target.CommandResult{Stdout: "old-name\n"}, nil
		default:
			return target.CommandResult{ExitCode: 1}, nil
		}
	}}

	op := &rebootLxcOp{pveCmd: pveCmd{id: 100}, hostname: "pihole"}
	result, drift, err := op.checkWith(context.Background(), cmdr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != spec.CheckUnsatisfied {
		t.Errorf("got %v, want CheckUnsatisfied", result)
	}
	if len(drift) != 1 || drift[0].Field != "hostname (reboot)" {
		t.Errorf("got drift %v, want hostname (reboot)", drift)
	}
}

func TestRebootOpCheck_ContainerStopped(t *testing.T) {
	cmdr := &mockTarget{handler: func(cmd string) (target.CommandResult, error) {
		switch cmd {
		case "pct status 100":
			return target.CommandResult{Stdout: "status: stopped\n"}, nil
		default:
			return target.CommandResult{ExitCode: 1}, nil
		}
	}}

	op := &rebootLxcOp{pveCmd: pveCmd{id: 100}, hostname: "pihole"}
	result, _, err := op.checkWith(context.Background(), cmdr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != spec.CheckSatisfied {
		t.Errorf("got %v, want CheckSatisfied (no reboot for stopped container)", result)
	}
}
