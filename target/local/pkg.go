// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"context"
	"fmt"
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/target"
)

func (t POSIXTarget) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	cmd := fmt.Sprintf(t.pkgBackend.IsInstalled, shellQuote(pkg))
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (t POSIXTarget) InstallPkgs(ctx context.Context, pkgs []string) error {
	if t.pkgBackend.NeedsRoot && !t.isRoot && t.escalate == "" {
		return target.NoEscalationError{Op: t.pkgBackend.Name + " install"}
	}
	quoted := make([]string, len(pkgs))
	for i, p := range pkgs {
		quoted[i] = shellQuote(p)
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

func (t POSIXTarget) RemovePkgs(ctx context.Context, pkgs []string) error {
	if t.pkgBackend.NeedsRoot && !t.isRoot && t.escalate == "" {
		return target.NoEscalationError{Op: t.pkgBackend.Name + " remove"}
	}
	quoted := make([]string, len(pkgs))
	for i, p := range pkgs {
		quoted[i] = shellQuote(p)
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

func (t POSIXTarget) UpdateCache(ctx context.Context) error {
	if !t.pkgBackend.SupportsUpgrade() {
		return errs.BUG(
			"%s backend does not support upgrade checks — capability should have prevented this call",
			t.pkgBackend.Name,
		)
	}
	if t.pkgBackend.CacheNeedsRoot && !t.isRoot && t.escalate == "" {
		return target.NoEscalationError{Op: t.pkgBackend.Name + " update-cache"}
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

func (t POSIXTarget) IsUpgradable(ctx context.Context, pkg string) (bool, error) {
	if !t.pkgBackend.SupportsUpgrade() {
		return false, errs.BUG(
			"%s backend does not support upgrade checks"+
				" — capability should have prevented this call",
			t.pkgBackend.Name,
		)
	}
	cmd := fmt.Sprintf(t.pkgBackend.IsUpgradable, shellQuote(pkg))
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
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
