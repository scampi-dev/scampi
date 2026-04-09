// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/rest"
)

// mapFields maps eval.Value fields onto a Go config struct pointer
// using reflection. Field names are matched by converting Go field
// names to snake_case.
func mapFields(fields map[string]eval.Value, cfg any, lc *linkConfig) error {
	v := reflect.ValueOf(cfg)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()

	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := toSnake(f.Name)
		// Keywords can't be field names — check common renames.
		if name == "type" {
			name = "fs_type"
		}
		val, ok := fields[name]
		if !ok {
			// Apply default from struct tag if present.
			if def := f.Tag.Get("default"); def != "" {
				applyDefault(v.Field(i), def)
			}
			continue
		}
		fv := v.Field(i)
		if err := setValue(fv, val, lc); err != nil {
			return err
		}
	}
	return nil
}

// setValue assigns an eval.Value to a reflect.Value.
func setValue(dst reflect.Value, src eval.Value, lc *linkConfig) error {
	if src == nil {
		return nil
	}
	// Interface fields (any): convert to Go native types.
	if dst.Kind() == reflect.Interface {
		dst.Set(reflect.ValueOf(evalToGo(src)))
		return nil
	}
	switch sv := src.(type) {
	case *eval.StringVal:
		switch {
		case dst.Kind() == reflect.String:
			dst.SetString(sv.V)
		case dst.Type() == reflect.TypeOf(target.Port{}):
			dst.Set(reflect.ValueOf(convertPort(sv.V)))
		case dst.Type() == reflect.TypeOf(target.Mount{}):
			dst.Set(reflect.ValueOf(convertMount(sv.V)))
		}
	case *eval.IntVal:
		switch dst.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			dst.SetInt(sv.V)
		}
	case *eval.BoolVal:
		if dst.Kind() == reflect.Bool {
			dst.SetBool(sv.V)
		}
	case *eval.ListVal:
		if dst.Kind() == reflect.Slice {
			slice := reflect.MakeSlice(dst.Type(), len(sv.Items), len(sv.Items))
			for i, item := range sv.Items {
				if err := setValue(slice.Index(i), item, lc); err != nil {
					return err
				}
			}
			dst.Set(slice)
		}
	case *eval.MapVal:
		switch dst.Kind() {
		case reflect.Map:
			m := reflect.MakeMap(dst.Type())
			for i, k := range sv.Keys {
				kv := reflect.New(dst.Type().Key()).Elem()
				if err := setValue(kv, k, lc); err != nil {
					return err
				}
				vv := reflect.New(dst.Type().Elem()).Elem()
				if err := setValue(vv, sv.Values[i], lc); err != nil {
					return err
				}
				m.SetMapIndex(kv, vv)
			}
			dst.Set(m)
		case reflect.Struct:
			// Map with string keys → struct fields by snake_case match.
			fields := make(map[string]eval.Value)
			for i, k := range sv.Keys {
				if sk, ok := k.(*eval.StringVal); ok {
					fields[sk.V] = sv.Values[i]
				}
			}
			if err := mapFields(fields, dst.Addr().Interface(), lc); err != nil {
				return err
			}
		case reflect.Interface:
			// Convert to Go map[string]any for interface{} fields.
			dst.Set(reflect.ValueOf(evalMapToGo(sv)))
		}
	case *eval.NoneVal:
		// Leave as zero value.
	case *eval.StructVal:
		// Composable types: convert StructVal to the Go type the
		// engine expects based on the destination field type.
		dstType := dst.Type()
		switch {
		case dstType == reflect.TypeOf(spec.SourceRef{}):
			dst.Set(reflect.ValueOf(convertSourceRef(sv, lc)))
		case dstType == reflect.TypeOf(spec.PkgSourceRef{}):
			dst.Set(reflect.ValueOf(convertPkgSourceRef(sv)))
		case dstType == reflect.TypeOf((*rest.AuthConfig)(nil)).Elem():
			dst.Set(reflect.ValueOf(convertAuth(sv)))
		case dstType == reflect.TypeOf((*rest.TLSConfig)(nil)).Elem():
			dst.Set(reflect.ValueOf(convertTLS(sv)))
		case dst.Kind() == reflect.Pointer && dstType.Elem() == reflect.TypeOf(target.Healthcheck{}):
			dst.Set(reflect.ValueOf(convertHealthcheck(sv)))
		case dst.Kind() == reflect.Interface:
			dst.Set(reflect.ValueOf(sv))
		case dst.Kind() == reflect.Pointer && dst.IsNil():
			ptr := reflect.New(dstType.Elem())
			if err := mapFields(sv.Fields, ptr.Interface(), lc); err != nil {
				return err
			}
			dst.Set(ptr)
		case dst.Kind() == reflect.Struct:
			if err := mapFields(sv.Fields, dst.Addr().Interface(), lc); err != nil {
				return err
			}
		}
	}
	return nil
}

// Composable type converters
// -----------------------------------------------------------------------------

func convertSourceRef(sv *eval.StructVal, lc *linkConfig) spec.SourceRef {
	ref := spec.SourceRef{}
	switch sv.TypeName {
	case "source_local":
		ref.Kind = spec.SourceLocal
		if p, ok := sv.Fields["path"].(*eval.StringVal); ok {
			ref.Path = p.V
		}
	case "source_inline":
		ref.Kind = spec.SourceInline
		if c, ok := sv.Fields["content"].(*eval.StringVal); ok {
			ref.Content = c.V
			// Write to cache so the engine can read the file.
			if lc != nil && lc.src != nil {
				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(c.V)))[:12]
				cachePath := filepath.Join(filepath.Dir(lc.cfgPath), ".scampi-cache", "inline-"+hash)
				_ = lc.src.WriteFile(lc.ctx, cachePath, []byte(c.V))
				ref.Path = cachePath
			}
		}
	case "source_remote":
		ref.Kind = spec.SourceRemote
		if u, ok := sv.Fields["url"].(*eval.StringVal); ok {
			ref.URL = u.V
		}
	}
	return ref
}

func convertPkgSourceRef(sv *eval.StructVal) spec.PkgSourceRef {
	ref := spec.PkgSourceRef{}
	switch sv.TypeName {
	case "pkg_system":
		ref.Kind = spec.PkgSourceNative
	case "pkg_apt_repo":
		ref.Kind = spec.PkgSourceApt
		if u, ok := sv.Fields["url"].(*eval.StringVal); ok {
			ref.URL = u.V
			ref.Name = repoSlug(u.V)
		}
		if k, ok := sv.Fields["key_url"].(*eval.StringVal); ok {
			ref.KeyURL = k.V
		}
		if c, ok := sv.Fields["components"].(*eval.ListVal); ok {
			for _, item := range c.Items {
				if s, ok := item.(*eval.StringVal); ok {
					ref.Components = append(ref.Components, s.V)
				}
			}
		}
		if s, ok := sv.Fields["suite"].(*eval.StringVal); ok {
			ref.Suite = s.V
		}
	case "pkg_dnf_repo":
		ref.Kind = spec.PkgSourceDnf
		if u, ok := sv.Fields["url"].(*eval.StringVal); ok {
			ref.URL = u.V
			ref.Name = repoSlug(u.V)
		}
		if k, ok := sv.Fields["key_url"].(*eval.StringVal); ok {
			ref.KeyURL = k.V
		}
	}
	return ref
}

// applyDefault sets a struct field to its default value from the tag.
func applyDefault(dst reflect.Value, def string) {
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(def)
	case reflect.Bool:
		dst.SetBool(def == "true")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v, err := strconv.ParseInt(def, 10, 64); err == nil {
			dst.SetInt(v)
		}
	}
}

// evalToGo converts an eval.Value to a Go native type (for any/interface fields).
func evalToGo(v eval.Value) any {
	switch sv := v.(type) {
	case *eval.StringVal:
		return sv.V
	case *eval.IntVal:
		return sv.V
	case *eval.BoolVal:
		return sv.V
	case *eval.NoneVal:
		return nil
	case *eval.ListVal:
		r := make([]any, len(sv.Items))
		for i, item := range sv.Items {
			r[i] = evalToGo(item)
		}
		return r
	case *eval.MapVal:
		return evalMapToGo(sv)
	}
	return nil
}

func evalMapToGo(mv *eval.MapVal) map[string]any {
	m := make(map[string]any, len(mv.Keys))
	for i, k := range mv.Keys {
		if sk, ok := k.(*eval.StringVal); ok {
			m[sk.V] = evalToGo(mv.Values[i])
		}
	}
	return m
}

func repoSlug(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		h := sha256.Sum256([]byte(rawURL))
		return hex.EncodeToString(h[:8])
	}
	host := strings.ReplaceAll(u.Hostname(), ".", "-")
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		return host + "-" + parts[0]
	}
	return host
}

// toSnake converts GoFieldName to snake_case.
func toSnake(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				prev := runes[i-1]
				nextIsLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
				if prev >= 'a' && prev <= 'z' {
					b.WriteByte('_')
				} else if prev >= 'A' && prev <= 'Z' && nextIsLower {
					b.WriteByte('_')
				}
			}
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
