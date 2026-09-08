// SPDX-License-Identifier: GPL-3.0-only

package controller

import (
	"context"
	"path/filepath"

	"scampi.dev/scampi/internal/errs"
)

// rooted wraps a Controller and resolves relative paths against a base directory.
// Absolute paths are passed through unchanged.
type rooted struct {
	base string
	src  Controller
}

// WithRoot creates a Controller that resolves relative paths against baseDir.
func WithRoot(root string, src Controller) Controller {
	base, err := baseDir(root, src)
	if err != nil {
		panic(errs.BUG("failed to find base-dir for %q: %w", root, err))
	}

	return &rooted{
		base: base,
		src:  src,
	}
}

func (r *rooted) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(r.base, path)
}

func (r *rooted) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return r.src.ReadFile(ctx, r.resolve(path))
}

func (r *rooted) WriteFile(ctx context.Context, path string, data []byte) error {
	return r.src.WriteFile(ctx, r.resolve(path), data)
}

func (r *rooted) EnsureDir(ctx context.Context, path string) error {
	return r.src.EnsureDir(ctx, r.resolve(path))
}

func (r *rooted) Stat(ctx context.Context, path string) (FileMeta, error) {
	return r.src.Stat(ctx, r.resolve(path))
}

func (r *rooted) LookupEnv(key string) (string, bool) {
	return r.src.LookupEnv(key)
}

func baseDir(p string, src Controller) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	info, err := src.Stat(context.Background(), abs)
	if err != nil {
		return "", err
	}

	if info.IsDir {
		return abs, nil
	}

	return filepath.Dir(abs), nil
}
