// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"fmt"
	"strings"

	"scampi.dev/scampi/diagnostic/event"
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

func (e PkgInstallError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeInstallFailed,
			Text: `failed to install pkgs [{{.Pkgs}}]`,
			Hint: `verify the package names in [{{.Pkgs}}] exist in the ` +
				`configured repos and the package cache is current`,
			Help:   `{{.Stderr}}`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

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

func (e PkgRemoveError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeRemoveFailed,
			Text:   `failed to remove pkgs [{{.Pkgs}}]`,
			Hint:   `confirm packages in [{{.Pkgs}}] are installed and not held by another package`,
			Help:   `{{.Stderr}}`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

// PkgCacheError is emitted when a package cache update fails.
type PkgCacheError struct {
	Stderr string
	Source spec.SourceSpan
}

func (e PkgCacheError) Error() string {
	return fmt.Sprintf("failed to update package cache: %s", e.Stderr)
}

func (e PkgCacheError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeCacheUpdateFailed,
			Text:   `failed to update package cache: {{.Stderr}}`,
			Hint:   "check network connectivity and package manager configuration",
			Help:   "the cache refresh command exited with a non-zero status",
			Data:   e,
			Source: &e.Source,
		},
	}
}

// RepoKeyInstallError is emitted when installing a repo signing key fails.
type RepoKeyInstallError struct {
	Name   string
	Detail string
}

func (e RepoKeyInstallError) Error() string {
	return fmt.Sprintf("failed to install signing key for %q: %s", e.Name, e.Detail)
}

func (e RepoKeyInstallError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeRepoKeyInstallFailed,
			Text: `failed to install signing key for "{{.Name}}"`,
			Hint: `verify the key_url for "{{.Name}}" is reachable from the target and serves a valid GPG key`,
			Help: `{{.Detail}}`,
			Data: e,
		},
	}
}

// RepoConfigError is emitted when writing a repo config fails.
type RepoConfigError struct {
	Name   string
	Detail string
}

func (e RepoConfigError) Error() string {
	return fmt.Sprintf("failed to configure repo %q: %s", e.Name, e.Detail)
}

func (e RepoConfigError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeRepoConfigFailed,
			Text: `failed to configure repo "{{.Name}}"`,
			Hint: `verify scampi can write the repo config for "{{.Name}}" ` +
				`(e.g. /etc/apt/sources.list.d/, /etc/yum.repos.d/)`,
			Help: `{{.Detail}}`,
			Data: e,
		},
	}
}

// SuiteDetectionError is emitted when the distro codename can't be determined.
type SuiteDetectionError struct {
	Name string
}

func (e SuiteDetectionError) Error() string {
	return fmt.Sprintf("could not detect suite for repo %q", e.Name)
}

func (e SuiteDetectionError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeSuiteDetectionFailed,
			Text: `could not detect suite (codename) for repo "{{.Name}}"`,
			Hint: `specify suite explicitly: apt_repo(url=..., key_url=..., suite="bookworm")`,
			Help: "the target's /etc/os-release did not contain VERSION_CODENAME",
			Data: e,
		},
	}
}

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

func (e SourceBackendMismatchError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeSourceBackendMismatch,
			Text:   `{{.SourceKind}} source cannot be used on a {{.TargetKind}} target`,
			Hint:   "use a source that matches the target's package manager",
			Data:   e,
			Source: &e.Source,
		},
	}
}
