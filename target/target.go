package target

import (
	"context"
	"io/fs"
)

type (
	// Target represents an execution environment.
	// Implementations define platform semantics (e.g. POSIX, Windows, remote).
	// Ops must treat Target as authoritative for system behavior.
	Target interface {
		Filesystem
		Ownership
	}
	Owner struct {
		User  string
		Group string
	}
	Filesystem interface {
		ReadFile(ctx context.Context, path string) ([]byte, error)
		WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error
		Stat(ctx context.Context, path string) (fs.FileInfo, error)

		Chown(ctx context.Context, path string, owner Owner) error
		Chmod(ctx context.Context, path string, mode fs.FileMode) error
	}
	Ownership interface {
		GetOwner(ctx context.Context, path string) (Owner, error)
	}
)
