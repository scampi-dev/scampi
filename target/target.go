package target

import (
	"context"
	"errors"
	"io/fs"

	"godoit.dev/doit/capability"
)

var (
	ErrNotExist     = errors.New("path does not exist")
	ErrUnknownUser  = errors.New("unknown user")
	ErrUnknownGroup = errors.New("unknown group")
)

type (
	// Target represents an execution environment.
	// Implementations define platform semantics (e.g. POSIX, Windows, remote).
	// Ops must treat Target as authoritative for system behavior.
	Target interface {
		Filesystem
		Ownership

		Capabilities() capability.Capability
	}
	Owner struct {
		User  string
		Group string
	}
	Filesystem interface {
		ReadFile(ctx context.Context, path string) ([]byte, error)
		WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error
		Stat(ctx context.Context, path string) (fs.FileInfo, error)
		Lstat(ctx context.Context, path string) (fs.FileInfo, error)
		Readlink(ctx context.Context, path string) (string, error)
		Symlink(ctx context.Context, target, link string) error
		Remove(ctx context.Context, path string) error

		Chown(ctx context.Context, path string, owner Owner) error
		Chmod(ctx context.Context, path string, mode fs.FileMode) error
	}
	Ownership interface {
		HasUser(ctx context.Context, user string) bool
		HasGroup(ctx context.Context, group string) bool
		GetOwner(ctx context.Context, path string) (Owner, error)
	}
)

func IsNotExist(err error) bool     { return errors.Is(err, ErrNotExist) }
func IsUnknownUser(err error) bool  { return errors.Is(err, ErrUnknownUser) }
func IsUnknownGroup(err error) bool { return errors.Is(err, ErrUnknownGroup) }
