// SPDX-License-Identifier: GPL-3.0-only

package controller

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

// Controller provides access to the filesystem and environment of the machine
// running scampi, where configs, templates, and secrets reside. Distinct from
// target.Target, which is the system being converged.
type Controller interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	EnsureDir(ctx context.Context, path string) error
	Stat(ctx context.Context, path string) (FileMeta, error)
	LookupEnv(key string) (string, bool)
}
