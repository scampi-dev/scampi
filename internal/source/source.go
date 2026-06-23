// SPDX-License-Identifier: GPL-3.0-only

package source

import (
	"context"
	"time"
)

type FileMeta struct {
	Exists   bool
	IsDir    bool
	Size     int64
	Modified time.Time
}

// Source provides access to the host filesystem and environment where
// scampi configs, templates, and secrets reside. Distinct from
// target.Target which represents the system being converged.
type Source interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	EnsureDir(ctx context.Context, path string) error
	Stat(ctx context.Context, path string) (FileMeta, error)
	LookupEnv(key string) (string, bool)
}
