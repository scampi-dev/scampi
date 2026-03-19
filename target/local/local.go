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
	"scampi.dev/scampi/target/posix"
	"scampi.dev/scampi/target/svcmgr"
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
	tgt.SvcBackend = svcmgr.Detect(func(cmd string) (int, error) {
		result, err := tgt.RunCommand(ctx, cmd)
		if err != nil {
			return -1, err
		}
		return result.ExitCode, nil
	})

	// Privilege escalation detection.
	tgt.IsRoot = os.Getuid() == 0
	tgt.Escalate = posix.DetectEscalation(ctx, tgt.RunCommand, tgt.IsRoot)

	return tgt, nil
}

type Config struct{}
