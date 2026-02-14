// SPDX-License-Identifier: GPL-3.0-only

package spec

import (
	"fmt"
	"reflect"
	"strings"
)

// DocFromConfig builds a full StepDoc from the struct tags on a config struct.
//
// The struct must have an unexported blank field with a `summary` tag for the
// step-level description. Every exported field must carry a `step` tag.
// Missing tags cause a panic so drift is caught immediately.
//
// Struct-level tag (on an unexported _ field):
//
//	summary:"Copy files with owner and permission management"
//
// Field-level tags (on exported fields):
//
//	step:"Description text"        — field description (required tag)
//	optional:"true"                — marks field as optional (default: required)
//	default:"present"              — display default (implies optional)
//	example:"/etc/app/config.yaml" — example value
func DocFromConfig(kind string, cfg any) StepDoc {
	rt := reflectStruct(cfg)
	return StepDoc{
		Kind:    kind,
		Summary: extractSummary(rt),
		Fields:  extractFields(rt),
	}
}

func reflectStruct(cfg any) reflect.Type {
	rv := reflect.ValueOf(cfg)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	rt := rv.Type()
	if rt.Kind() != reflect.Struct {
		panic(fmt.Sprintf("DocFromConfig: expected struct, got %s", rt.Kind()))
	}
	return rt
}

func extractSummary(rt reflect.Type) string {
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if sf.IsExported() {
			continue
		}
		if v, ok := sf.Tag.Lookup("summary"); ok {
			return v
		}
	}
	panic(fmt.Sprintf(
		"DocFromConfig(%s): no unexported field with summary tag",
		rt.Name(),
	))
}

func extractFields(rt reflect.Type) []FieldDoc {
	fields := make([]FieldDoc, 0, rt.NumField())
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}

		desc, ok := sf.Tag.Lookup("step")
		if !ok {
			panic(fmt.Sprintf(
				"DocFromConfig(%s): field %q has no step tag",
				rt.Name(), sf.Name,
			))
		}

		fd := FieldDoc{
			Name:     strings.ToLower(sf.Name),
			Type:     goKindToDocType(sf.Type),
			Desc:     desc,
			Required: true,
		}

		if sf.Tag.Get("optional") == "true" {
			fd.Required = false
		}
		if v := sf.Tag.Get("default"); v != "" {
			fd.Required = false
			fd.Default = `"` + v + `"`
		}
		if v := sf.Tag.Get("example"); v != "" {
			fd.Example = v
		}

		fields = append(fields, fd)
	}
	return fields
}

func goKindToDocType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Bool:
		return "bool"
	case reflect.Slice:
		return "list"
	case reflect.Map, reflect.Struct:
		return "struct"
	default:
		return t.Kind().String()
	}
}
