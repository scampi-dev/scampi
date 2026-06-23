// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"reflect"
	"strconv"
	"strings"

	"scampi.dev/scampi/internal/lang/eval"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
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
		name := ToSnake(f.Name)
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
	// RefVal: preserve as-is in any-typed fields so resolveStepRefs
	// can find and convert them to spec.Ref after StepID assignment.
	if rv, ok := src.(*eval.RefVal); ok {
		if dst.Kind() == reflect.Interface {
			dst.Set(reflect.ValueOf(rv))
			return nil
		}
	}
	// StructVal needs type-specific handling first — check before
	// the generic interface path.
	if sv, ok := src.(*eval.StructVal); ok {
		return setStructVal(dst, sv, lc)
	}
	// Interface fields: plain `any` fields get Go-native conversion
	// (map[string]any, []any, scalars) so template data etc. work
	// with Go's text/template engine. Narrower interfaces like
	// eval.Value preserve the original eval type so downstream
	// consumers (testkit target constructors) can read the shape.
	if dst.Kind() == reflect.Interface {
		if dst.Type() != reflect.TypeFor[any]() {
			dst.Set(reflect.ValueOf(src))
			return nil
		}
		goVal := evalToGo(src)
		if goVal != nil {
			dst.Set(reflect.ValueOf(goVal))
		}
		return nil
	}
	switch sv := src.(type) {
	case *eval.StringVal:
		switch {
		case dst.Kind() == reflect.String:
			dst.SetString(sv.V)
		case dst.Type() == reflect.TypeFor[target.Port]():
			dst.Set(reflect.ValueOf(convertPort(sv.V)))
		case dst.Type() == reflect.TypeFor[target.Mount]():
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
		// Handled by setStructVal above — should not reach here.
	}
	return nil
}

// setStructVal handles StructVal → Go type conversion.
func setStructVal(dst reflect.Value, sv *eval.StructVal, lc *linkConfig) error {
	dstType := dst.Type()

	// Check registered type converters first.
	if lc.converterFor != nil {
		if converter, ok := lc.converterFor(dstType); ok {
			cc := spec.ConvertContext{
				CfgPath: lc.cfgPath,
				Src:     lc.src,
				Ctx:     lc.ctx,
			}
			result, err := converter(sv.TypeName, sv.Fields, cc)
			if err != nil {
				return err
			}
			if result != nil {
				dst.Set(reflect.ValueOf(result))
			}
			return nil
		}
	}

	// Generic fallback for interfaces, pointers, and plain structs.
	switch {
	case dst.Kind() == reflect.Interface:
		if dstType == reflect.TypeFor[any]() {
			dst.Set(reflect.ValueOf(structValToMap(sv)))
		} else {
			dst.Set(reflect.ValueOf(sv))
		}
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
	return nil
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
	case *eval.RefVal:
		// Preserve RefVals through Go-native conversion so
		// resolveMapRefs can find and convert them to spec.Ref
		// after StepID assignment.
		return sv
	}
	return nil
}

func structValToMap(sv *eval.StructVal) map[string]any {
	m := make(map[string]any, len(sv.Fields))
	for k, v := range sv.Fields {
		m[k] = evalToGo(v)
	}
	return m
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

// ToSnake converts GoFieldName to snake_case. Exported because the
// stub-drift lint in test/stub_drift_test.go must apply the exact
// same name conversion the linker uses when mapping evaluated scampi
// fields onto Go config structs.
func ToSnake(s string) string {
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
