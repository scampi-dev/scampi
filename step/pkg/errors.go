package pkg

import (
	"fmt"
	"strings"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

// PkgInstallError is emitted when a package install command fails.
type PkgInstallError struct {
	Pkgs     []string
	Stderr   string
	ExitCode int
	Source   spec.SourceSpan
}

func (e PkgInstallError) Error() string {
	return fmt.Sprintf("failed to install pkgs [%s] (exit %d)", strings.Join(e.Pkgs, ", "), e.ExitCode)
}

func (e PkgInstallError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.pkg.InstallFailed",
		Text:   `failed to install pkgs [{{.Pkgs}}]: {{.Stderr}}`,
		Hint:   "check that the package names are correct and the package manager cache is up to date",
		Help:   "the package manager command exited with a non-zero status",
		Data:   e,
		Source: &e.Source,
	}
}

func (PkgInstallError) Severity() signal.Severity { return signal.Error }
func (PkgInstallError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// PkgRemoveError is emitted when a package remove command fails.
type PkgRemoveError struct {
	Pkgs     []string
	Stderr   string
	ExitCode int
	Source   spec.SourceSpan
}

func (e PkgRemoveError) Error() string {
	return fmt.Sprintf("failed to remove pkgs [%s] (exit %d)", strings.Join(e.Pkgs, ", "), e.ExitCode)
}

func (e PkgRemoveError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.pkg.RemoveFailed",
		Text:   `failed to remove pkgs [{{.Pkgs}}]: {{.Stderr}}`,
		Hint:   "check that the packages are installed and can be removed",
		Help:   "the package manager command exited with a non-zero status",
		Data:   e,
		Source: &e.Source,
	}
}

func (PkgRemoveError) Severity() signal.Severity { return signal.Error }
func (PkgRemoveError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
