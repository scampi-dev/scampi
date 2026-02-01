package target

import (
	"context"
	"errors"
	"io/fs"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/errs"
)

var (
	ErrNotExist     = errors.New("path does not exist")
	ErrPermission   = errors.New("permission denied")
	ErrUnknownUser  = errors.New("unknown user")
	ErrUnknownGroup = errors.New("unknown group")
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
)

func IsNotExist(err error) bool     { return errors.Is(err, ErrNotExist) }
func IsPermission(err error) bool   { return errors.Is(err, ErrPermission) }
func IsUnknownUser(err error) bool  { return errors.Is(err, ErrUnknownUser) }
func IsUnknownGroup(err error) bool { return errors.Is(err, ErrUnknownGroup) }

func Must[T any](reqID string, tgt Target) T {
	res, ok := tgt.(T)
	if !ok {
		panic(errs.BUG("%s requires Filesystem capable target, got %T", reqID, tgt))
	}
	return res
}
