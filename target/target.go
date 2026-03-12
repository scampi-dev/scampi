// SPDX-License-Identifier: GPL-3.0-only

package target

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
)

var (
	ErrNotExist        = errors.New("path does not exist")
	ErrPermission      = errors.New("permission denied")
	ErrUnknownUser     = errors.New("unknown user")
	ErrUnknownGroup    = errors.New("unknown group")
	ErrCommandNotFound = errors.New("command not found on target")
)

type (
	// Target represents an execution environment.
	// Implementations define platform semantics (e.g. POSIX, Windows, remote).
	// Ops must treat Target as authoritative for system behavior.
	Target interface {
		Capabilities() capability.Capability
	}
	Closer interface {
		Close()
	}
	Owner struct {
		User  string
		Group string
	}
	Filesystem interface {
		Stat(ctx context.Context, path string) (fs.FileInfo, error)
		ReadFile(ctx context.Context, path string) ([]byte, error)
		WriteFile(ctx context.Context, path string, data []byte) error
		Remove(ctx context.Context, path string) error
		Mkdir(ctx context.Context, path string, mode fs.FileMode) error
	}
	FileMode interface {
		Chmod(ctx context.Context, path string, mode fs.FileMode) error
	}
	Symlink interface {
		Lstat(ctx context.Context, path string) (fs.FileInfo, error)
		Readlink(ctx context.Context, path string) (string, error)
		Symlink(ctx context.Context, target, link string) error
	}
	Ownership interface {
		HasUser(ctx context.Context, user string) bool
		HasGroup(ctx context.Context, group string) bool
		GetOwner(ctx context.Context, path string) (Owner, error)
		Chown(ctx context.Context, path string, owner Owner) error
	}
	PkgManager interface {
		IsInstalled(ctx context.Context, pkg string) (bool, error)
		InstallPkgs(ctx context.Context, pkgs []string) error
		RemovePkgs(ctx context.Context, pkgs []string) error
	}
	PkgUpdater interface {
		UpdateCache(ctx context.Context) error
		IsUpgradable(ctx context.Context, pkg string) (bool, error)
	}
	ServiceManager interface {
		IsActive(ctx context.Context, name string) (bool, error)
		IsEnabled(ctx context.Context, name string) (bool, error)
		Start(ctx context.Context, name string) error
		Stop(ctx context.Context, name string) error
		Enable(ctx context.Context, name string) error
		Disable(ctx context.Context, name string) error
		DaemonReload(ctx context.Context) error
		Restart(ctx context.Context, name string) error
		Reload(ctx context.Context, name string) error
		SupportsReload() bool
	}
	CommandResult struct {
		Stdout   string
		Stderr   string
		ExitCode int
	}
	Command interface {
		RunCommand(ctx context.Context, cmd string) (CommandResult, error)
	}
)

func IsNotExist(err error) bool        { return errors.Is(err, ErrNotExist) }
func IsPermission(err error) bool      { return errors.Is(err, ErrPermission) }
func IsUnknownUser(err error) bool     { return errors.Is(err, ErrUnknownUser) }
func IsUnknownGroup(err error) bool    { return errors.Is(err, ErrUnknownGroup) }
func IsCommandNotFound(err error) bool { return errors.Is(err, ErrCommandNotFound) }

func Must[T any](reqID string, tgt Target) T {
	res, ok := tgt.(T)
	if !ok {
		panic(errs.BUG("%s requires %T capable target, got %T", reqID, (*T)(nil), tgt))
	}
	return res
}

// SvcCommandError is returned when a service management command fails.
type SvcCommandError struct {
	Op       string
	Name     string
	Stderr   string
	ExitCode int
}

func (e SvcCommandError) Error() string {
	return fmt.Sprintf("service %s %s failed (exit %d): %s", e.Op, e.Name, e.ExitCode, e.Stderr)
}

// EscalationError is returned when a privilege-escalated command fails
// (password required, not in sudoers, command denied, etc).
type EscalationError struct {
	Tool     string // "sudo" or "doas"
	Op       string // "cat", "cp", "rm", "chmod", "chown", "ln"
	Path     string
	Stderr   string
	ExitCode int
}

func (e EscalationError) Error() string {
	stderr := strings.TrimSpace(e.Stderr)
	if stderr != "" {
		return fmt.Sprintf(
			"%s %s %s: exit %d: %s",
			e.Tool, e.Op, e.Path, e.ExitCode, stderr,
		)
	}
	return fmt.Sprintf(
		"%s %s %s: exit %d",
		e.Tool, e.Op, e.Path, e.ExitCode,
	)
}

// NoEscalationError is returned when an operation requires root
// but the user is not root and no escalation tool (sudo/doas) was found.
type NoEscalationError struct {
	Op   string // "read", "write", "chmod", "apk install", …
	Path string
}

func (e NoEscalationError) Error() string {
	return fmt.Sprintf("%s %s: no escalation tool found (sudo/doas)", e.Op, e.Path)
}

// StagingError is returned when writing a temp file for
// escalated copy fails.
type StagingError struct {
	Path string // destination path the staged file was for
	Err  error
}

func (e StagingError) Error() string {
	return fmt.Sprintf("stage temp file for %s: %s", e.Path, e.Err)
}

func (e StagingError) Unwrap() error { return e.Err }
