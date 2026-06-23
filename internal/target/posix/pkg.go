// SPDX-License-Identifier: GPL-3.0-only

package posix

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/target"
)

// PkgManager
// -----------------------------------------------------------------------------

func (b Base) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	cmd := fmt.Sprintf(b.PkgBackend.IsInstalled, target.ShellQuote(pkg))
	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (b Base) InstallPkgs(ctx context.Context, pkgs []string) error {
	if b.PkgBackend.NeedsRoot && b.NeedsEscalation() {
		return b.NoEscalation(b.PkgBackend.Kind.String()+" install", "")
	}
	quoted := make([]string, len(pkgs))
	for i, p := range pkgs {
		quoted[i] = target.ShellQuote(p)
	}
	cmd := fmt.Sprintf(b.PkgBackend.Install, strings.Join(quoted, " "))
	if b.PkgBackend.NeedsRoot && b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}
	result, err := b.Runner(ctx, cmd)
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

func (b Base) RemovePkgs(ctx context.Context, pkgs []string) error {
	if b.PkgBackend.NeedsRoot && b.NeedsEscalation() {
		return b.NoEscalation(b.PkgBackend.Kind.String()+" remove", "")
	}
	quoted := make([]string, len(pkgs))
	for i, p := range pkgs {
		quoted[i] = target.ShellQuote(p)
	}
	cmd := fmt.Sprintf(b.PkgBackend.Remove, strings.Join(quoted, " "))
	if b.PkgBackend.NeedsRoot && b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}
	result, err := b.Runner(ctx, cmd)
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

// PkgUpdater
// -----------------------------------------------------------------------------

func (b Base) UpdateCache(ctx context.Context) error {
	if !b.PkgBackend.SupportsUpgrade() {
		return errs.BUG(
			"%s backend does not support upgrade checks — capability should have prevented this call",
			b.PkgBackend.Kind.String(),
		)
	}
	if b.PkgBackend.CacheNeedsRoot && b.NeedsEscalation() {
		return b.NoEscalation(b.PkgBackend.Kind.String()+" update-cache", "")
	}
	cmd := b.PkgBackend.UpdateCache
	if b.PkgBackend.CacheNeedsRoot && b.Escalate != "" {
		cmd = b.Escalate + " " + cmd
	}
	result, err := b.Runner(ctx, cmd)
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

func (b Base) IsUpgradable(ctx context.Context, pkg string) (bool, error) {
	if !b.PkgBackend.SupportsUpgrade() {
		return false, errs.BUG(
			"%s backend does not support upgrade checks"+
				" — capability should have prevented this call",
			b.PkgBackend.Kind.String(),
		)
	}
	cmd := fmt.Sprintf(b.PkgBackend.IsUpgradable, target.ShellQuote(pkg))
	result, err := b.Runner(ctx, cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (b Base) CacheAge(ctx context.Context) (time.Duration, error) {
	if b.PkgBackend.CheckCacheAge == "" {
		return 0, target.ErrNoCacheInfo
	}
	result, err := b.Runner(ctx, b.PkgBackend.CheckCacheAge)
	if err != nil || result.ExitCode != 0 {
		return 0, target.ErrNoCacheInfo
	}
	epoch, err := strconv.ParseInt(strings.TrimSpace(result.Stdout), 10, 64)
	if err != nil {
		return 0, target.ErrNoCacheInfo
	}
	return time.Since(time.Unix(epoch, 0)), nil
}

// Errors
// -----------------------------------------------------------------------------

type PkgInstallError struct {
	Pkgs     []string
	Stderr   string
	ExitCode int
}

func (e PkgInstallError) Error() string {
	return fmt.Sprintf("pkg install failed (exit %d): %s", e.ExitCode, e.Stderr)
}

type PkgRemoveError struct {
	Pkgs     []string
	Stderr   string
	ExitCode int
}

func (e PkgRemoveError) Error() string {
	return fmt.Sprintf("pkg remove failed (exit %d): %s", e.ExitCode, e.Stderr)
}

type CacheUpdateError struct {
	Stderr   string
	ExitCode int
}

func (e CacheUpdateError) Error() string {
	return fmt.Sprintf("pkg cache update failed (exit %d): %s", e.ExitCode, e.Stderr)
}
