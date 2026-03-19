// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/target"
)

func (t *SSHTarget) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	cmd := fmt.Sprintf(t.pkgBackend.IsInstalled, target.ShellQuote(pkg))
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (t *SSHTarget) InstallPkgs(ctx context.Context, pkgs []string) error {
	if t.pkgBackend.NeedsRoot && !t.isRoot && t.escalate == "" {
		return target.NoEscalationError{Op: t.pkgBackend.Kind.String() + " install"}
	}
	quoted := make([]string, len(pkgs))
	for i, p := range pkgs {
		quoted[i] = target.ShellQuote(p)
	}
	cmd := fmt.Sprintf(t.pkgBackend.Install, strings.Join(quoted, " "))
	if t.pkgBackend.NeedsRoot && t.escalate != "" {
		cmd = t.escalate + " " + cmd
	}
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return PkgInstallError{
			Pkgs:     pkgs,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t *SSHTarget) RemovePkgs(ctx context.Context, pkgs []string) error {
	if t.pkgBackend.NeedsRoot && !t.isRoot && t.escalate == "" {
		return target.NoEscalationError{Op: t.pkgBackend.Kind.String() + " remove"}
	}
	quoted := make([]string, len(pkgs))
	for i, p := range pkgs {
		quoted[i] = target.ShellQuote(p)
	}
	cmd := fmt.Sprintf(t.pkgBackend.Remove, strings.Join(quoted, " "))
	if t.pkgBackend.NeedsRoot && t.escalate != "" {
		cmd = t.escalate + " " + cmd
	}
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return PkgRemoveError{
			Pkgs:     pkgs,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t *SSHTarget) UpdateCache(ctx context.Context) error {
	if !t.pkgBackend.SupportsUpgrade() {
		return errs.BUG(
			"%s backend does not support upgrade checks"+
				" — capability should have prevented this call",
			t.pkgBackend.Kind.String(),
		)
	}
	if t.pkgBackend.CacheNeedsRoot && !t.isRoot && t.escalate == "" {
		return target.NoEscalationError{Op: t.pkgBackend.Kind.String() + " update-cache"}
	}
	cmd := t.pkgBackend.UpdateCache
	if t.pkgBackend.CacheNeedsRoot && t.escalate != "" {
		cmd = t.escalate + " " + cmd
	}
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return CacheUpdateError{
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t *SSHTarget) IsUpgradable(ctx context.Context, pkg string) (bool, error) {
	if !t.pkgBackend.SupportsUpgrade() {
		return false, errs.BUG(
			"%s backend does not support upgrade checks"+
				" — capability should have prevented this call",
			t.pkgBackend.Kind.String(),
		)
	}
	cmd := fmt.Sprintf(t.pkgBackend.IsUpgradable, target.ShellQuote(pkg))
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (t *SSHTarget) CacheAge(ctx context.Context) (time.Duration, error) {
	if t.pkgBackend.CheckCacheAge == "" {
		return 0, target.ErrNoCacheInfo
	}
	result, err := t.RunCommand(ctx, t.pkgBackend.CheckCacheAge)
	if err != nil || result.ExitCode != 0 {
		return 0, target.ErrNoCacheInfo
	}
	epoch, err := strconv.ParseInt(strings.TrimSpace(result.Stdout), 10, 64)
	if err != nil {
		return 0, target.ErrNoCacheInfo
	}
	return time.Since(time.Unix(epoch, 0)), nil
}

// PkgInstallError is returned when a package install command fails.
type PkgInstallError struct {
	Pkgs     []string
	Stderr   string
	ExitCode int
}

func (e PkgInstallError) Error() string {
	return fmt.Sprintf("pkg install failed (exit %d): %s", e.ExitCode, e.Stderr)
}

// PkgRemoveError is returned when a package remove command fails.
type PkgRemoveError struct {
	Pkgs     []string
	Stderr   string
	ExitCode int
}

func (e PkgRemoveError) Error() string {
	return fmt.Sprintf("pkg remove failed (exit %d): %s", e.ExitCode, e.Stderr)
}

// CacheUpdateError is returned when a package cache refresh command fails.
type CacheUpdateError struct {
	Stderr   string
	ExitCode int
}

func (e CacheUpdateError) Error() string {
	return fmt.Sprintf("pkg cache update failed (exit %d): %s", e.ExitCode, e.Stderr)
}
