// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

type fileKind struct{}

func (fileKind) Identify() Identity { return Identity{"path"} }

func (fileKind) Validate(r Resource) error {
	var errs []error
	if !r.Has("path") {
		errs = append(errs, fmt.Errorf("%s: missing required attr %q", r.Ref(), "path"))
	}
	if !r.Has("content") {
		errs = append(errs, fmt.Errorf("%s: missing required attr %q", r.Ref(), "content"))
	}
	return errors.Join(errs...)
}

func (fileKind) Check(_ context.Context, r Resource) (State, error) {
	path := r.Attrs["path"]
	content := r.Attrs["content"]
	current, err := os.ReadFile(path)
	switch {
	case err == nil && string(current) == content:
		return StateMatching, nil
	case err == nil:
		return StateDiverging, nil
	case errors.Is(err, fs.ErrNotExist):
		return StateMissing, nil
	}
	return 0, fmt.Errorf("%s: read %s: %w", r.Ref(), path, err)
}

func (fileKind) Apply(ctx context.Context, r Resource, log Log) error {
	ref := r.Ref()
	path := r.Attrs["path"]
	content := r.Attrs["content"]
	log.Emit(ctx, CodeApplyStart, &ref, "path", path)
	log.Info(ctx, "writing file", "ref", ref, "path", path)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		err = fmt.Errorf("%s: write %s: %w", ref, path, err)
		log.Emit(ctx, CodeApplyFailed, &ref, "path", path, "err", err)
		return err
	}
	return nil
}

func (fileKind) Destroy(ctx context.Context, ref Ref, attrs Attrs, log Log) error {
	path := attrs["path"]
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		log.Emit(ctx, CodeDestroySuccess, &ref, "path", path)
		return nil
	}
	log.Emit(ctx, CodeDestroyStart, &ref, "path", path)
	log.Info(ctx, "removing file", "ref", ref, "path", path)
	if err := os.Remove(path); err != nil {
		err = fmt.Errorf("%s: remove %s: %w", ref, path, err)
		log.Emit(ctx, CodeDestroyFailed, &ref, "path", path, "err", err)
		return err
	}
	log.Emit(ctx, CodeDestroySuccess, &ref, "path", path)
	return nil
}
