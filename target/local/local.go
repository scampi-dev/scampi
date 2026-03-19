// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"context"
	"os"
	"strings"

	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/pkgmgr"
	"scampi.dev/scampi/target/svcmgr"
)

type Local struct{}

func (Local) Kind() string   { return "local" }
func (Local) NewConfig() any { return &Config{} }
func (Local) Create(ctx context.Context, _ source.Source, _ spec.TargetInstance) (target.Target, error) {
	tgt := &POSIXTarget{}

	// OS detection for package manager support.
	// Phase 1: kernel via uname -s
	var osInfo pkgmgr.OSInfo
	if result, err := tgt.RunCommand(ctx, "uname -s"); err == nil {
		osInfo.Kernel = strings.TrimSpace(result.Stdout)
	}

	// Phase 2: distro detection (Linux only) via /etc/os-release
	if osInfo.Kernel == pkgmgr.KernelLinux {
		if content, err := tgt.ReadFile(ctx, "/etc/os-release"); err == nil {
			osInfo = pkgmgr.ParseOSRelease(content)
			osInfo.Kernel = pkgmgr.KernelLinux
		}
	}

	tgt.osInfo = osInfo
	tgt.pkgBackend = pkgmgr.Detect(osInfo)

	// Init system detection for service management.
	tgt.svcBackend = svcmgr.Detect(func(cmd string) (int, error) {
		result, err := tgt.RunCommand(ctx, cmd)
		if err != nil {
			return -1, err
		}
		return result.ExitCode, nil
	})

	// Privilege escalation detection.
	tgt.isRoot = os.Getuid() == 0
	tgt.escalate = detectEscalation(ctx, tgt)

	return tgt, nil
}

func detectEscalation(ctx context.Context, tgt *POSIXTarget) string {
	if tgt.isRoot {
		return ""
	}
	if result, err := tgt.RunCommand(ctx, "command -v sudo"); err == nil && result.ExitCode == 0 {
		return "sudo"
	}
	if result, err := tgt.RunCommand(ctx, "command -v doas"); err == nil && result.ExitCode == 0 {
		return "doas"
	}
	return ""
}

type Config struct{}
