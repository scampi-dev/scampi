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
	if !hasAttr(r, "path") {
		errs = append(errs, fmt.Errorf("%s: missing required attr %q", r.Ref(), "path"))
	}
	if !hasAttr(r, "content") {
		errs = append(errs, fmt.Errorf("%s: missing required attr %q", r.Ref(), "content"))
	}
	return errors.Join(errs...)
}

func (fileKind) Apply(ctx context.Context, r Resource, log Log) (bool, error) {
	// path + content guaranteed present by Validate
	ref := r.Ref()
	path := r.Attrs["path"]
	content := r.Attrs["content"]
	current, err := os.ReadFile(path)
	switch {
	case err == nil && string(current) == content:
		log.Debug(ctx, "file in sync", "ref", ref, "path", path)
		return true, nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		err = fmt.Errorf("%s: read %s: %w", ref, path, err)
		log.Emit(ctx, CodeApplyFailed, &ref, "path", path, "err", err)
		return false, err
	}
	log.Emit(ctx, CodeApplyStart, &ref, "path", path)
	log.Info(ctx, "writing file", "ref", ref, "path", path)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		err = fmt.Errorf("%s: write %s: %w", ref, path, err)
		log.Emit(ctx, CodeApplyFailed, &ref, "path", path, "err", err)
		return false, err
	}
	return false, nil
}

func (fileKind) Destroy(ctx context.Context, ref Ref, attrs map[string]string, log Log) error {
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
