// SPDX-License-Identifier: GPL-3.0-only

package posix

import (
	"context"
	"fmt"
	"io/fs"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/pkgmgr"
	"scampi.dev/scampi/target/svcmgr"
)

// Base holds state shared by all POSIX target implementations and provides
// method implementations for package management, service management, and
// user/group management. Transport-specific types (local, SSH) embed this
// and set Runner to their command execution function.
type Base struct {
	Runner     func(ctx context.Context, cmd string) (target.CommandResult, error)
	OSInfo     target.OSInfo
	PkgBackend *pkgmgr.Backend
	SvcBackend svcmgr.Backend
	Escalate   string
	IsRoot     bool
}

// NeedsEscalation reports whether a privileged operation would fail
// because the current user is not root and no escalation tool is available.
func (b Base) NeedsEscalation() bool {
	return !b.IsRoot && b.Escalate == ""
}

func (b Base) Capabilities() capability.Capability {
	caps := capability.POSIX
	if b.PkgBackend != nil {
		caps |= capability.Pkg
		if b.PkgBackend.SupportsUpgrade() {
			caps |= capability.PkgUpdate
		}
		if b.PkgBackend.SupportsRepoSetup() {
			caps |= capability.PkgRepo
		}
	}
	if b.SvcBackend != nil {
		caps |= capability.Service
	}
	return caps
}

func (b Base) RunPrivileged(ctx context.Context, cmd string) (target.CommandResult, error) {
	if b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}
	return b.Runner(ctx, cmd)
}

func (b Base) VersionCodename() string {
	return b.OSInfo.VersionCodename
}

func (b Base) ChmodRecursive(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := "chmod -R " + octal + " " + target.ShellQuote(path)
	if b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}
	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: b.Escalate, Op: "chmod -R", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (b Base) ChownRecursive(ctx context.Context, path string, owner target.Owner) error {
	cmd := "chown -R " +
		target.ShellQuote(owner.User) + ":" + target.ShellQuote(owner.Group) +
		" " + target.ShellQuote(path)
	if b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}
	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: b.Escalate, Op: "chown -R", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

// DetectEscalation probes for sudo/doas on the target.
func DetectEscalation(
	ctx context.Context,
	run func(context.Context, string) (target.CommandResult, error),
	isRoot bool,
) string {
	if isRoot {
		return ""
	}
	if result, err := run(ctx, "command -v sudo"); err == nil && result.ExitCode == 0 {
		return "sudo"
	}
	if result, err := run(ctx, "command -v doas"); err == nil && result.ExitCode == 0 {
		return "doas"
	}
	return ""
}
