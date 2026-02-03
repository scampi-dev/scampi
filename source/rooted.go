package source

import (
	"context"
	"path/filepath"

	"godoit.dev/doit/errs"
)

// rootedSource wraps a Source and resolves relative paths against a base directory.
// Absolute paths are passed through unchanged.
type rootedSource struct {
	base string
	src  Source
}

// WithRoot creates a Source that resolves relative paths against baseDir.
func WithRoot(root string, src Source) Source {
	base, err := baseDir(root, src)
	if err != nil {
		panic(errs.BUG("failed to find base-dir for %q: %w", root, err))
	}

	return &rootedSource{
		base: base,
		src:  src,
	}
}

func (r *rootedSource) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(r.base, path)
}

func (r *rootedSource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return r.src.ReadFile(ctx, r.resolve(path))
}

func (r *rootedSource) WriteFile(ctx context.Context, path string, data []byte) error {
	return r.src.WriteFile(ctx, r.resolve(path), data)
}

func (r *rootedSource) EnsureDir(ctx context.Context, path string) error {
	return r.src.EnsureDir(ctx, r.resolve(path))
}

func (r *rootedSource) Stat(ctx context.Context, path string) (FileMeta, error) {
	return r.src.Stat(ctx, r.resolve(path))
}

func (r *rootedSource) LookupEnv(key string) (string, bool) {
	return r.src.LookupEnv(key)
}

func baseDir(p string, src Source) (string, error) {
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
