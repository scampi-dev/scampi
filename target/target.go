// SPDX-License-Identifier: GPL-3.0-only

package target

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
)

// bare-error: target sentinels, checked via errors.Is by step code
var (
	ErrNotExist        = errs.New("path does not exist")
	ErrPermission      = errs.New("permission denied")
	ErrUnknownUser     = errs.New("unknown user")
	ErrUnknownGroup    = errs.New("unknown group")
	ErrCommandNotFound = errs.New("command not found on target")
	ErrNoCacheInfo     = errs.New("package cache age not available")
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
		ReadDir(ctx context.Context, path string) ([]fs.DirEntry, error)
		ReadFile(ctx context.Context, path string) ([]byte, error)
		WriteFile(ctx context.Context, path string, data []byte) error
		Remove(ctx context.Context, path string) error
		Mkdir(ctx context.Context, path string, mode fs.FileMode) error
	}
	FileMode interface {
		Chmod(ctx context.Context, path string, mode fs.FileMode) error
		ChmodRecursive(ctx context.Context, path string, mode fs.FileMode) error
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
		ChownRecursive(ctx context.Context, path string, owner Owner) error
	}
	PkgManager interface {
		IsInstalled(ctx context.Context, pkg string) (bool, error)
		InstallPkgs(ctx context.Context, pkgs []string) error
		RemovePkgs(ctx context.Context, pkgs []string) error
	}
	PkgUpdater interface {
		UpdateCache(ctx context.Context) error
		IsUpgradable(ctx context.Context, pkg string) (bool, error)
		CacheAge(ctx context.Context) (time.Duration, error)
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
	Mount struct {
		Source   string // host path (absolute)
		Target   string // container path (absolute)
		ReadOnly bool
	}
	Healthcheck struct {
		Cmd         string
		Interval    time.Duration
		Timeout     time.Duration
		Retries     int
		StartPeriod time.Duration
	}
	PortProto uint8
	Port      struct {
		HostIP        string    // bind address ("", "127.0.0.1", etc.)
		HostPort      string    // host port number
		ContainerPort string    // container port number
		Proto         PortProto // protocol (default: TCP)
	}
)

func (m Mount) String() string {
	s := m.Source + ":" + m.Target
	if m.ReadOnly {
		s += ":ro"
	}
	return s
}

const (
	ProtoTCP PortProto = iota
	ProtoUDP
)

func ParsePortProto(s string) PortProto {
	if s == "udp" {
		return ProtoUDP
	}
	return ProtoTCP
}

func (p PortProto) String() string {
	if p == ProtoUDP {
		return "udp"
	}
	return "tcp"
}

func (p Port) String() string {
	s := ""
	if p.HostIP != "" {
		s += p.HostIP + ":"
	}
	s += p.HostPort + ":" + p.ContainerPort
	if p.Proto != ProtoTCP {
		s += "/" + p.Proto.String()
	}
	return s
}

// Flag returns the value for the -p flag passed to docker/podman create.
func (p Port) Flag() string {
	return p.String()
}

type (
	ContainerInfo struct {
		Name         string
		Image        string
		Running      bool
		Restart      string
		Ports        []Port
		Env          map[string]string
		Mounts       []Mount
		Args         []string
		Labels       map[string]string
		Healthcheck  *Healthcheck
		HealthStatus string // "", "starting", "healthy", "unhealthy"
	}
	ContainerManager interface {
		InspectContainer(ctx context.Context, name string) (ContainerInfo, bool, error)
		CreateContainer(ctx context.Context, opts ContainerInfo) error
		StartContainer(ctx context.Context, name string) error
		StopContainer(ctx context.Context, name string) error
		RemoveContainer(ctx context.Context, name string) error
	}
	CommandResult struct {
		Stdout   string
		Stderr   string
		ExitCode int
	}
	Command interface {
		RunCommand(ctx context.Context, cmd string) (CommandResult, error)
		RunPrivileged(ctx context.Context, cmd string) (CommandResult, error)
	}
	UserInfo struct {
		Name     string
		UID      int
		GID      int
		Home     string
		Shell    string
		Groups   []string // supplementary group names
		System   bool
		Password string
	}
	GroupInfo struct {
		Name    string
		GID     int
		System  bool
		Members []string
	}
	UserManager interface {
		UserExists(ctx context.Context, name string) (bool, error)
		GetUser(ctx context.Context, name string) (UserInfo, error)
		CreateUser(ctx context.Context, info UserInfo) error
		ModifyUser(ctx context.Context, info UserInfo) error
		DeleteUser(ctx context.Context, name string) error
	}
	GroupManager interface {
		GroupExists(ctx context.Context, name string) (bool, error)
		GetGroup(ctx context.Context, name string) (GroupInfo, error)
		CreateGroup(ctx context.Context, info GroupInfo) error
		DeleteGroup(ctx context.Context, name string) error
	}
	RepoConfig struct {
		Name       string // slug for filenames
		URL        string
		KeyData    []byte   // downloaded+dearmored key content (apt) or raw key (dnf)
		KeyPath    string   // deterministic path on target
		ConfigPath string   // deterministic path on target
		Suite      string   // apt: codename
		Components []string // apt: components list
	}
	RepoManager interface {
		HasRepo(ctx context.Context, name string) (bool, error)
		HasRepoKey(ctx context.Context, name string) (bool, error)
		InstallRepoKey(ctx context.Context, cfg RepoConfig) error
		WriteRepoConfig(ctx context.Context, cfg RepoConfig) error
		RemoveRepo(ctx context.Context, name string) error
		RepoKeyPath(name string) string
		RepoConfigPath(name string) string
	}
	OSInfoProvider interface {
		VersionCodename() string
	}
	HTTPRequest struct {
		Method  string
		Path    string
		Headers map[string]string
		Body    []byte
	}
	HTTPResponse struct {
		StatusCode int
		Headers    map[string][]string
		Body       []byte
	}
	HTTPClient interface {
		Do(ctx context.Context, req HTTPRequest) (*HTTPResponse, error)
	}
	// Traceable is an optional interface for targets that collect
	// transport-level trace messages (e.g. auth lifecycle events).
	Traceable interface {
		DrainTraces() []string
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

// ShellQuote single-quotes a string for safe shell interpolation.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
