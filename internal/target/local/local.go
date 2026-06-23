// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"context"
	"os"
	"strings"

	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/ctrmgr"
	"scampi.dev/scampi/internal/target/pkgmgr"
	"scampi.dev/scampi/internal/target/posix"
	"scampi.dev/scampi/internal/target/svcmgr"
)

type Local struct{}

func (Local) Kind() string   { return "local" }
func (Local) NewConfig() any { return &Config{} }
func (Local) Create(ctx context.Context, _ source.Source, _ spec.TargetInstance) (target.Target, error) {
	tgt := &POSIXTarget{}
	tgt.Runner = tgt.RunCommand

	// OS detection for package manager and platform-specific dispatch.
	var osInfo target.OSInfo
	if result, err := tgt.RunCommand(ctx, "uname -s"); err == nil {
		kernel := strings.TrimSpace(result.Stdout)
		osInfo.Platform = target.ParseKernel(kernel)

		// Linux needs distro detection via /etc/os-release.
		if kernel == "Linux" {
			if content, err := tgt.ReadFile(ctx, "/etc/os-release"); err == nil {
				osInfo = target.ResolveLinuxPlatform(content)
			}
		}
	}

	tgt.OSInfo = osInfo
	tgt.PkgBackend = pkgmgr.Detect(osInfo.Platform)

	// Init system detection for service management.
	detectCmd := func(cmd string) (int, error) {
		result, err := tgt.RunCommand(ctx, cmd)
		if err != nil {
			return -1, err
		}
		return result.ExitCode, nil
	}
	tgt.SvcBackend = svcmgr.Detect(detectCmd)
	tgt.CtrBackend = ctrmgr.Detect(detectCmd)

	// Privilege escalation detection.
	tgt.IsRoot = os.Getuid() == 0
	tgt.Escalate, tgt.EscalateReason = posix.DetectEscalation(ctx, tgt.RunCommand, tgt.IsRoot)

	return tgt, nil
}

type Config struct{}
