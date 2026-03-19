// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"fmt"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
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

// PkgCacheError is emitted when a package cache update fails.
type PkgCacheError struct {
	Stderr string
	Source spec.SourceSpan
}

func (e PkgCacheError) Error() string {
	return fmt.Sprintf("failed to update package cache: %s", e.Stderr)
}

func (e PkgCacheError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.pkg.CacheUpdateFailed",
		Text:   `failed to update package cache: {{.Stderr}}`,
		Hint:   "check network connectivity and package manager configuration",
		Help:   "the cache refresh command exited with a non-zero status",
		Data:   e,
		Source: &e.Source,
	}
}

func (PkgCacheError) Severity() signal.Severity { return signal.Error }
func (PkgCacheError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// EmptyPackagesError is raised when packages list is empty.
type EmptyPackagesError struct {
	Source spec.SourceSpan
}

func (e EmptyPackagesError) Error() string {
	return "packages must not be empty"
}

func (e EmptyPackagesError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.pkg.EmptyPackages",
		Text:   "packages must not be empty",
		Hint:   "provide at least one package name",
		Data:   e,
		Source: &e.Source,
	}
}

func (EmptyPackagesError) Severity() signal.Severity { return signal.Error }
func (EmptyPackagesError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// InvalidStateError is raised when the state field has an unrecognized value.
type InvalidStateError struct {
	Got     string
	Allowed []string
	Source  spec.SourceSpan
}

func (e InvalidStateError) Error() string {
	return fmt.Sprintf("invalid state %q", e.Got)
}

func (e InvalidStateError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.pkg.InvalidState",
		Text:   `invalid state "{{.Got}}"`,
		Hint:   `expected one of: {{join ", " .Allowed}}`,
		Data:   e,
		Source: &e.Source,
	}
}

func (InvalidStateError) Severity() signal.Severity { return signal.Error }
func (InvalidStateError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// RepoKeyInstallError is emitted when installing a repo signing key fails.
type RepoKeyInstallError struct {
	Name   string
	Detail string
}

func (e RepoKeyInstallError) Error() string {
	return fmt.Sprintf("failed to install signing key for %q: %s", e.Name, e.Detail)
}

func (e RepoKeyInstallError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.pkg.RepoKeyInstallFailed",
		Text: `failed to install signing key for "{{.Name}}": {{.Detail}}`,
		Hint: "check that the key URL is correct and the target is reachable",
		Data: e,
	}
}

func (RepoKeyInstallError) Severity() signal.Severity { return signal.Error }
func (RepoKeyInstallError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// RepoConfigError is emitted when writing a repo config fails.
type RepoConfigError struct {
	Name   string
	Detail string
}

func (e RepoConfigError) Error() string {
	return fmt.Sprintf("failed to configure repo %q: %s", e.Name, e.Detail)
}

func (e RepoConfigError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.pkg.RepoConfigFailed",
		Text: `failed to configure repo "{{.Name}}": {{.Detail}}`,
		Hint: "check target write permissions for the package manager config directory",
		Data: e,
	}
}

func (RepoConfigError) Severity() signal.Severity { return signal.Error }
func (RepoConfigError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// SuiteDetectionError is emitted when the distro codename can't be determined.
type SuiteDetectionError struct {
	Name string
}

func (e SuiteDetectionError) Error() string {
	return fmt.Sprintf("could not detect suite for repo %q", e.Name)
}

func (e SuiteDetectionError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.pkg.SuiteDetectionFailed",
		Text: `could not detect suite (codename) for repo "{{.Name}}"`,
		Hint: `specify suite explicitly: apt_repo(url=..., key_url=..., suite="bookworm")`,
		Help: "the target's /etc/os-release did not contain VERSION_CODENAME",
		Data: e,
	}
}

func (SuiteDetectionError) Severity() signal.Severity { return signal.Error }
func (SuiteDetectionError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// SourceBackendMismatchError is emitted when the source type doesn't match
// the target's package manager (e.g. apt_repo on a dnf system).
type SourceBackendMismatchError struct {
	SourceKind string
	TargetKind string
	Source     spec.SourceSpan
}

func (e SourceBackendMismatchError) Error() string {
	return fmt.Sprintf("source %s cannot be used on %s target", e.SourceKind, e.TargetKind)
}

func (e SourceBackendMismatchError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.pkg.SourceBackendMismatch",
		Text:   `{{.SourceKind}} source cannot be used on a {{.TargetKind}} target`,
		Hint:   "use a source that matches the target's package manager",
		Data:   e,
		Source: &e.Source,
	}
}

func (SourceBackendMismatchError) Severity() signal.Severity { return signal.Error }
func (SourceBackendMismatchError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
