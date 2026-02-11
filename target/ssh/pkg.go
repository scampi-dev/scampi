package ssh

import (
	"context"
	"fmt"
	"strings"
)

func (t *SSHTarget) IsInstalled(_ context.Context, pkg string) (bool, error) {
	cmd := fmt.Sprintf(t.pkgBackend.IsInstalled, shellQuote(pkg))
	result, err := t.runCommand(cmd)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (t *SSHTarget) InstallPkgs(_ context.Context, pkgs []string) error {
	quoted := make([]string, len(pkgs))
	for i, p := range pkgs {
		quoted[i] = shellQuote(p)
	}
	cmd := fmt.Sprintf(t.pkgBackend.Install, strings.Join(quoted, " "))
	result, err := t.runCommand(cmd)
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

func (t *SSHTarget) RemovePkgs(_ context.Context, pkgs []string) error {
	quoted := make([]string, len(pkgs))
	for i, p := range pkgs {
		quoted[i] = shellQuote(p)
	}
	cmd := fmt.Sprintf(t.pkgBackend.Remove, strings.Join(quoted, " "))
	result, err := t.runCommand(cmd)
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
