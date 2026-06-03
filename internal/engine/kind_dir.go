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

func (dirKind) Identify() Identity { return Identity{"path"} }

func (dirKind) Validate(r Resource) error {
	if !r.Has("path") {
		return fmt.Errorf("%s: missing required attr %q", r.Ref(), "path")
	}
	return nil
}

func (dirKind) Check(_ context.Context, r Resource) (State, error) {
	path := r.Attrs["path"]
	info, err := os.Stat(path)
	switch {
	case err == nil && info.IsDir():
		return StateMatching, nil
	case err == nil:
		return StateDiverging, nil
	case errors.Is(err, fs.ErrNotExist):
		return StateMissing, nil
	}
	return 0, fmt.Errorf("%s: stat %s: %w", r.Ref(), path, err)
}

func (dirKind) Apply(ctx context.Context, r Resource, log Log) error {
	ref := r.Ref()
	path := r.Attrs["path"]
	log.Emit(ctx, CodeApplyStart, &ref, "path", path)
	log.Info(ctx, "creating dir", "ref", ref, "path", path)
	if err := os.MkdirAll(path, 0o755); err != nil {
		err = fmt.Errorf("%s: mkdir %s: %w", ref, path, err)
		log.Emit(ctx, CodeApplyFailed, &ref, "path", path, "err", err)
		return err
	}
	return nil
}

// Destroy uses os.Remove, which refuses non-empty directories.
// Non-empty stays orphaned until the operator clears it.
func (dirKind) Destroy(ctx context.Context, ref Ref, attrs Attrs, log Log) error {
	path := attrs["path"]
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		log.Emit(ctx, CodeDestroySuccess, &ref, "path", path)
		return nil
	}
	log.Emit(ctx, CodeDestroyStart, &ref, "path", path)
	log.Info(ctx, "removing dir", "ref", ref, "path", path)
	if err := os.Remove(path); err != nil {
		err = fmt.Errorf("%s: remove %s: %w", ref, path, err)
		log.Emit(ctx, CodeDestroyFailed, &ref, "path", path, "err", err)
		return err
	}
	log.Emit(ctx, CodeDestroySuccess, &ref, "path", path)
	return nil
}
