// SPDX-License-Identifier: GPL-3.0-only

package sharedop

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/spec"
)

// Converters returns the type converters owned by sharedop.
func Converters() spec.ConverterMap {
	return spec.ConverterMap{
		reflect.TypeFor[spec.SourceRef](): ConvertSourceRef,
	}
}

// ConvertSourceRef converts a StructVal produced by source_local,
// source_inline, or source_remote into a spec.SourceRef.
func ConvertSourceRef(typeName string, fields map[string]eval.Value, ctx spec.ConvertContext) (any, error) {
	ref := spec.SourceRef{}
	switch typeName {
	case "source_local":
		ref.Kind = spec.SourceLocal
		if p, ok := fields["path"].(*eval.StringVal); ok {
			ref.Path = p.V
		}
	case "source_inline":
		ref.Kind = spec.SourceInline
		if c, ok := fields["content"].(*eval.StringVal); ok {
			ref.Content = c.V
			if ctx.Src != nil {
				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(c.V)))[:12]
				cacheDir := filepath.Join(filepath.Dir(ctx.CfgPath), ".scampi-cache")
				cachePath := filepath.Join(cacheDir, "inline-"+hash)
				_ = ctx.Src.EnsureDir(ctx.Ctx, cacheDir)
				_ = ctx.Src.WriteFile(ctx.Ctx, cachePath, []byte(c.V))
				ref.Path = cachePath
			}
		}
	case "source_remote":
		ref.Kind = spec.SourceRemote
		if u, ok := fields["url"].(*eval.StringVal); ok {
			ref.URL = u.V
			if ctx.CfgPath != "" {
				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(u.V)))[:12]
				cacheDir := filepath.Join(filepath.Dir(ctx.CfgPath), ".scampi-cache")
				ref.Path = filepath.Join(cacheDir, "remote-"+hash)
			}
		}
	case "source_target":
		ref.Kind = spec.SourceTarget
		if p, ok := fields["path"].(*eval.StringVal); ok {
			ref.Path = p.V
		}
	}
	return ref, nil
}
