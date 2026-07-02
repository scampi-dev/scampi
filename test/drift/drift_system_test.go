// SPDX-License-Identifier: GPL-3.0-only

package drift

import (
	"testing"

	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/container"
	"scampi.dev/scampi/internal/step/firewall"
	"scampi.dev/scampi/internal/step/group"
	"scampi.dev/scampi/internal/step/mount"
	"scampi.dev/scampi/internal/step/service"
	"scampi.dev/scampi/internal/step/user"
	"scampi.dev/scampi/internal/target"
)

// ensureActiveOp / ensureEnabledOp
// -----------------------------------------------------------------------------

func Test_Drift_ReportsStoppedService(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Services["nginx"] = false
	tgt.EnabledServices["nginx"] = true

	ops := planOps(t, service.Service{}, &service.ServiceConfig{
		Name: "nginx", State: "running", Enabled: true,
	}, map[string]spec.FieldSpan{
		"name":    {},
		"state":   {},
		"enabled": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "stopped", "running")
}

func Test_Drift_ReportsDisabledService(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = false

	ops := planOps(t, service.Service{}, &service.ServiceConfig{
		Name: "nginx", State: "running", Enabled: true,
	}, map[string]spec.FieldSpan{
		"name":    {},
		"state":   {},
		"enabled": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "enabled", "disabled", "enabled")
}

func Test_Drift_ReportsRunningServiceWhenWantStopped(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Services["nginx"] = true
	tgt.EnabledServices["nginx"] = false

	ops := planOps(t, service.Service{}, &service.ServiceConfig{
		Name: "nginx", State: "stopped", Enabled: false,
	}, map[string]spec.FieldSpan{
		"name":    {},
		"state":   {},
		"enabled": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "running", "stopped")
}

// ensureUserOp
// -----------------------------------------------------------------------------

func Test_Drift_ReportsMissingUser(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	ops := planOps(t, user.User{}, &user.UserConfig{
		Name: "app", State: "present",
	}, map[string]spec.FieldSpan{
		"name":  {},
		"state": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "", "present")
}

func Test_Drift_ReportsUserShellDiff(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Users["app"] = target.UserInfo{Name: "app", Shell: "/bin/sh"}

	ops := planOps(t, user.User{}, &user.UserConfig{
		Name: "app", State: "present", Shell: "/bin/bash",
	}, map[string]spec.FieldSpan{
		"name":  {},
		"state": {},
		"shell": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "shell", "/bin/sh", "/bin/bash")
}

func Test_Drift_ReportsUserPresentWhenWantAbsent(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Users["app"] = target.UserInfo{Name: "app", Shell: "/bin/sh"}

	ops := planOps(t, user.User{}, &user.UserConfig{
		Name: "app", State: "absent",
	}, map[string]spec.FieldSpan{
		"name":  {},
		"state": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "present", "absent")
}

// ensureGroupOp / removeGroupOp
// -----------------------------------------------------------------------------

func Test_Drift_ReportsMissingGroup(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	ops := planOps(t, group.Group{}, &group.GroupConfig{
		Name: "web", State: "present",
	}, map[string]spec.FieldSpan{
		"name":  {},
		"state": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "", "present")
}

func Test_Drift_ReportsGroupPresentWhenWantAbsent(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Groups["web"] = target.GroupInfo{Name: "web"}

	ops := planOps(t, group.Group{}, &group.GroupConfig{
		Name: "web", State: "absent",
	}, map[string]spec.FieldSpan{
		"name":  {},
		"state": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "present", "absent")
}

// firewall ensureRuleOp
// -----------------------------------------------------------------------------

// firewallMemTarget scripts a ufw backend: "ufw version" succeeds so
// backend detection picks ufw, and "ufw show added" returns the given
// rule listing.
func firewallMemTarget(showAdded string) *target.MemTarget {
	tgt := target.NewMemTarget()
	tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
		switch cmd {
		case "ufw version":
			return target.CommandResult{Stdout: "ufw 0.36.2"}, nil
		case "ufw show added":
			return target.CommandResult{Stdout: showAdded}, nil
		}
		return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
	}
	return tgt
}

func Test_Drift_ReportsMissingFirewallRule(t *testing.T) {
	src := source.NewMemSource()
	tgt := firewallMemTarget("Added user rules (see 'ufw status' for running firewall):\n")

	ops := planOps(t, firewall.Firewall{}, &firewall.FirewallConfig{
		Port: 22, Proto: "tcp", Action: "allow",
	}, map[string]spec.FieldSpan{
		"port":   {},
		"proto":  {},
		"action": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "rule", "(absent)", "allow 22/tcp")
}

func Test_Drift_ReportsNoDriftWhenFirewallRulePresent(t *testing.T) {
	src := source.NewMemSource()
	tgt := firewallMemTarget(
		"Added user rules (see 'ufw status' for running firewall):\n" +
			"ufw allow 22/tcp\n",
	)

	ops := planOps(t, firewall.Firewall{}, &firewall.FirewallConfig{
		Port: 22, Proto: "tcp", Action: "allow",
	}, map[string]spec.FieldSpan{
		"port":   {},
		"proto":  {},
		"action": {},
	})

	details := collectDrift(t, ops, src, tgt)
	if len(details) != 0 {
		t.Errorf("expected no drift, got %+v", details)
	}
}

func Test_Drift_ReportsMissingFirewallPortRange(t *testing.T) {
	src := source.NewMemSource()
	tgt := firewallMemTarget("Added user rules (see 'ufw status' for running firewall):\n")

	ops := planOps(t, firewall.Firewall{}, &firewall.FirewallConfig{
		Port: 6000, EndPort: 6007, Proto: "udp", Action: "deny",
	}, map[string]spec.FieldSpan{
		"port":     {},
		"end_port": {},
		"proto":    {},
		"action":   {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "rule", "(absent)", "deny 6000:6007/udp")
}

// ensureMountOp
// -----------------------------------------------------------------------------

// mountMemTarget seeds /etc/fstab and scripts findmnt for /mnt/data:
// exit 0 when mounted, exit 1 when not.
func mountMemTarget(fstab string, mounted bool) *target.MemTarget {
	tgt := target.NewMemTarget()
	tgt.Files["/etc/fstab"] = []byte(fstab)
	tgt.CommandFunc = func(cmd string) (target.CommandResult, error) {
		if cmd == "findmnt --target /mnt/data --noheadings" {
			if mounted {
				return target.CommandResult{Stdout: "/mnt/data /dev/sdb1 ext4 rw\n"}, nil
			}
			return target.CommandResult{ExitCode: 1}, nil
		}
		return target.CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
	}
	return tgt
}

func Test_Drift_ReportsMissingFstabEntry(t *testing.T) {
	src := source.NewMemSource()
	tgt := mountMemTarget("# /etc/fstab: static file system information\n", false)

	ops := planOps(t, mount.Mount{}, &mount.MountConfig{
		Src:  "/dev/sdb1",
		Dest: "/mnt/data",
		Type: "ext4",
		Opts: "defaults",
		// mount.Plan panics on an empty state - Go-side construction
		// gets no stub defaults, so it must be set explicitly.
		State: "mounted",
	}, map[string]spec.FieldSpan{
		"src":   {},
		"dest":  {},
		"type":  {},
		"opts":  {},
		"state": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "fstab", "", "present")
	assertDrift(t, details, "mounted", "no", "yes")
}

func Test_Drift_ReportsFstabEntryDiff(t *testing.T) {
	src := source.NewMemSource()
	tgt := mountMemTarget("/dev/sdb1 /mnt/data ext4 noatime 0 0\n", true)

	ops := planOps(t, mount.Mount{}, &mount.MountConfig{
		Src:   "/dev/sdb1",
		Dest:  "/mnt/data",
		Type:  "ext4",
		Opts:  "defaults",
		State: "mounted",
	}, map[string]spec.FieldSpan{
		"src":   {},
		"dest":  {},
		"type":  {},
		"opts":  {},
		"state": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(
		t,
		details,
		"fstab",
		"/dev/sdb1 /mnt/data ext4 noatime 0 0",
		"/dev/sdb1 /mnt/data ext4 defaults 0 0",
	)
}

func Test_Drift_ReportsNoDriftWhenMountConverged(t *testing.T) {
	src := source.NewMemSource()
	tgt := mountMemTarget("/dev/sdb1 /mnt/data ext4 defaults 0 0\n", true)

	ops := planOps(t, mount.Mount{}, &mount.MountConfig{
		Src:   "/dev/sdb1",
		Dest:  "/mnt/data",
		Type:  "ext4",
		Opts:  "defaults",
		State: "mounted",
	}, map[string]spec.FieldSpan{
		"src":   {},
		"dest":  {},
		"type":  {},
		"opts":  {},
		"state": {},
	})

	details := collectDrift(t, ops, src, tgt)
	if len(details) != 0 {
		t.Errorf("expected no drift, got %+v", details)
	}
}

// ensureContainerOp
// -----------------------------------------------------------------------------

func Test_Drift_ReportsMissingContainer(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	ops := planOps(t, container.Instance{}, &container.InstanceConfig{
		Name:    "web",
		Image:   "nginx:1.25",
		State:   "running",
		Restart: "unless_stopped",
	}, map[string]spec.FieldSpan{
		"name":    {},
		"image":   {},
		"state":   {},
		"restart": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "(absent)", "running")
}

func Test_Drift_ReportsContainerImageDiff(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["web"] = target.ContainerInfo{
		Name:    "web",
		Image:   "nginx:1.24",
		Running: true,
		// Matches the desired policy so only image drifts.
		Restart: "unless-stopped",
	}

	ops := planOps(t, container.Instance{}, &container.InstanceConfig{
		Name:    "web",
		Image:   "nginx:1.25",
		State:   "running",
		Restart: "unless_stopped",
	}, map[string]spec.FieldSpan{
		"name":    {},
		"image":   {},
		"state":   {},
		"restart": {},
	})

	details := collectDrift(t, ops, src, tgt)
	if len(details) != 1 {
		t.Errorf("expected exactly one drift detail, got %+v", details)
	}
	assertDrift(t, details, "image", "nginx:1.24", "nginx:1.25")
}

func Test_Drift_ReportsStoppedContainer(t *testing.T) {
	src := source.NewMemSource()
	tgt := target.NewMemTarget()
	tgt.Containers["web"] = target.ContainerInfo{
		Name:    "web",
		Image:   "nginx:1.25",
		Running: false,
		Restart: "unless-stopped",
	}

	ops := planOps(t, container.Instance{}, &container.InstanceConfig{
		Name:    "web",
		Image:   "nginx:1.25",
		State:   "running",
		Restart: "unless_stopped",
	}, map[string]spec.FieldSpan{
		"name":    {},
		"image":   {},
		"state":   {},
		"restart": {},
	})

	details := collectDrift(t, ops, src, tgt)
	assertDrift(t, details, "state", "stopped", "running")
}
