// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"fmt"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// PkgInstallError is emitted when a package install command fails.
type PkgInstallError struct {
	diagnostic.FatalError
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

// PkgRemoveError is emitted when a package remove command fails.
type PkgRemoveError struct {
	diagnostic.FatalError
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

// PkgCacheError is emitted when a package cache update fails.
type PkgCacheError struct {
	diagnostic.FatalError
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

// RepoKeyInstallError is emitted when installing a repo signing key fails.
type RepoKeyInstallError struct {
	diagnostic.FatalError
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

// RepoConfigError is emitted when writing a repo config fails.
type RepoConfigError struct {
	diagnostic.FatalError
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

// SuiteDetectionError is emitted when the distro codename can't be determined.
type SuiteDetectionError struct {
	diagnostic.FatalError
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

// SourceBackendMismatchError is emitted when the source type doesn't match
// the target's package manager (e.g. apt_repo on a dnf system).
type SourceBackendMismatchError struct {
	diagnostic.FatalError
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
