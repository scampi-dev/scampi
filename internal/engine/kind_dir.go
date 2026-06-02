// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

type dirKind struct{}

func (dirKind) Validate(r Resource) error {
	if !hasAttr(r, "path") {
		return fmt.Errorf("%s: missing required attr %q", r.Ref(), "path")
	}
	return nil
}

func (dirKind) Apply(ctx context.Context, r Resource, log Log) (bool, error) {
	ref := r.Ref()
	path := r.Attrs["path"]
	info, serr := os.Stat(path)
	switch {
	case serr == nil && info.IsDir():
		log.Debug(ctx, "dir in sync", "ref", ref, "path", path)
		return true, nil
	case serr == nil:
		err := fmt.Errorf("%s: %s exists but is not a directory", ref, path)
		log.Emit(ctx, CodeApplyFailed, &ref, "path", path, "err", err)
		return false, err
	case !errors.Is(serr, fs.ErrNotExist):
		err := fmt.Errorf("%s: stat %s: %w", ref, path, serr)
		log.Emit(ctx, CodeApplyFailed, &ref, "path", path, "err", err)
		return false, err
	}
	log.Emit(ctx, CodeApplyStart, &ref, "path", path)
	log.Info(ctx, "creating dir", "ref", ref, "path", path)
	if merr := os.MkdirAll(path, 0o755); merr != nil {
		err := fmt.Errorf("%s: mkdir %s: %w", ref, path, merr)
		log.Emit(ctx, CodeApplyFailed, &ref, "path", path, "err", err)
		return false, err
	}
	return false, nil
}

// Destroy uses os.Remove, which refuses non-empty directories. That
// refusal becomes destroy.failed and the orphan stays in the inventory
// until the operator either clears the dir or accepts manual cleanup.
func (dirKind) Destroy(ctx context.Context, ref Ref, attrs map[string]string, log Log) error {
	path := attrs["path"]
	if _, serr := os.Stat(path); errors.Is(serr, fs.ErrNotExist) {
		log.Emit(ctx, CodeDestroySuccess, &ref, "path", path)
		return nil
	}
	log.Emit(ctx, CodeDestroyStart, &ref, "path", path)
	log.Info(ctx, "removing dir", "ref", ref, "path", path)
	if rerr := os.Remove(path); rerr != nil {
		err := fmt.Errorf("%s: remove %s: %w", ref, path, rerr)
		log.Emit(ctx, CodeDestroyFailed, &ref, "path", path, "err", err)
		return err
	}
	log.Emit(ctx, CodeDestroySuccess, &ref, "path", path)
	return nil
}
