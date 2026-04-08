// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"reflect"
	"strings"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/spec"
)

// mapFields maps eval.Value fields onto a Go config struct pointer
// using reflection. Field names are matched by converting Go field
// names to snake_case.
func mapFields(fields map[string]eval.Value, cfg any) error {
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
		val, ok := fields[name]
		if !ok {
			continue
		}
		fv := v.Field(i)
		if err := setValue(fv, val); err != nil {
			return err
		}
	}
	return nil
}

// setValue assigns an eval.Value to a reflect.Value.
func setValue(dst reflect.Value, src eval.Value) error {
	if src == nil {
		return nil
	}
	switch sv := src.(type) {
	case *eval.StringVal:
		if dst.Kind() == reflect.String {
			dst.SetString(sv.V)
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
				if err := setValue(slice.Index(i), item); err != nil {
					return err
				}
			}
			dst.Set(slice)
		}
	case *eval.MapVal:
		if dst.Kind() == reflect.Map {
			m := reflect.MakeMap(dst.Type())
			for i, k := range sv.Keys {
				kv := reflect.New(dst.Type().Key()).Elem()
				if err := setValue(kv, k); err != nil {
					return err
				}
				vv := reflect.New(dst.Type().Elem()).Elem()
				if err := setValue(vv, sv.Values[i]); err != nil {
					return err
				}
				m.SetMapIndex(kv, vv)
			}
			dst.Set(m)
		}
	case *eval.NoneVal:
		// Leave as zero value.
	case *eval.StructVal:
		// Composable types: convert StructVal to the Go type the
		// engine expects based on the destination field type.
		dstType := dst.Type()
		switch {
		case dstType == reflect.TypeOf(spec.SourceRef{}):
			dst.Set(reflect.ValueOf(convertSourceRef(sv)))
		case dstType == reflect.TypeOf(spec.PkgSourceRef{}):
			dst.Set(reflect.ValueOf(convertPkgSourceRef(sv)))
		case dst.Kind() == reflect.Interface:
			dst.Set(reflect.ValueOf(sv))
		case dst.Kind() == reflect.Struct:
			if err := mapFields(sv.Fields, dst.Addr().Interface()); err != nil {
				return err
			}
		}
	}
	return nil
}

// Composable type converters
// -----------------------------------------------------------------------------

func convertSourceRef(sv *eval.StructVal) spec.SourceRef {
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
		}
		if k, ok := sv.Fields["key_url"].(*eval.StringVal); ok {
			ref.KeyURL = k.V
		}
	}
	return ref
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
