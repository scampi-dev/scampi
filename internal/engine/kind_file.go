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

func (fileKind) Apply(ctx context.Context, r Resource, log Log) error {
	// path + content guaranteed present by Validate
	ref := r.Ref()
	path := r.Attrs["path"]
	content := r.Attrs["content"]
	current, rerr := os.ReadFile(path)
	switch {
	case rerr == nil && string(current) == content:
		log.Debug(ctx, "file in sync", "ref", ref, "path", path)
		return nil
	case rerr != nil && !errors.Is(rerr, fs.ErrNotExist):
		err := fmt.Errorf("%s: read %s: %w", ref, path, rerr)
		log.Emit(ctx, CodeApplyFailed, &ref, "path", path, "err", err)
		return err
	}
	log.Emit(ctx, CodeApplyStart, &ref, "path", path)
	log.Info(ctx, "writing file", "ref", ref, "path", path)
	if werr := os.WriteFile(path, []byte(content), 0o644); werr != nil {
		err := fmt.Errorf("%s: write %s: %w", ref, path, werr)
		log.Emit(ctx, CodeApplyFailed, &ref, "path", path, "err", err)
		return err
	}
	log.Emit(ctx, CodeApplySuccess, &ref, "path", path)
	return nil
}
