// SPDX-License-Identifier: GPL-3.0-only

// Package langstubs generates scampi-lang stub files from Go step
// config structs via reflection. It imports only stdlib — the caller
// provides concrete config struct pointers.
package langstubs

import (
	"io"
	"reflect"
	"strings"
)

// StubInput describes one step to generate a stub for.
type StubInput struct {
	Kind       string
	Config     any
	OutputType string
	Enums      map[string][]string
}

// Generate writes scampi-lang stub declarations for all inputs to w.
func Generate(inputs []StubInput, w io.Writer) error {
	bw := &builder{w: w}

	bw.line("# Auto-generated from Go struct tags. Do not edit.")
	bw.line("")

	emitted := map[string]bool{}
	for _, in := range inputs {
		for fieldName, variants := range in.Enums {
			name := enumTypeName(in.Kind, fieldName)
			if emitted[name] {
				continue
			}
			emitted[name] = true
			bw.line("enum " + name + " { " + strings.Join(variants, ", ") + " }")
		}
	}
	if len(emitted) > 0 {
		bw.line("")
	}

	for i, in := range inputs {
		if i > 0 {
			bw.line("")
		}
		emitStep(bw, in)
	}

	return bw.err
}

func emitStep(bw *builder, in StubInput) {
	t := reflect.TypeOf(in.Config)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	for f := range t.Fields() {
		if summary := f.Tag.Get("summary"); summary != "" {
			bw.line("# " + summary)
			break
		}
	}

	var fields []fieldInfo
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		stepTag := f.Tag.Get("step")
		if stepTag == "" {
			continue
		}
		name := toSnake(f.Name)
		if name == "desc" {
			continue
		}
		fi := fieldInfo{
			name:     name,
			goType:   f.Type,
			optional: f.Tag.Get("optional") == "true",
			defVal:   f.Tag.Get("default"),
		}
		if _, ok := in.Enums[f.Name]; ok {
			fi.enumName = enumTypeName(in.Kind, f.Name)
		}
		fields = append(fields, fi)
	}

	fields = append(fields,
		fieldInfo{name: "desc", scampiType: "string?", optional: true},
		fieldInfo{name: "on_change", scampiType: "list[StepInstance]", optional: true, defVal: "[]"},
	)

	bw.write("step " + in.Kind + "(")
	for i, f := range fields {
		if i > 0 {
			bw.write(", ")
		}
		typStr := f.resolveType()
		if f.optional && !strings.HasSuffix(typStr, "?") {
			typStr += "?"
		}
		entry := f.name + ": " + typStr
		if f.defVal != "" {
			entry += " = " + formatDefault(f.defVal, f.enumName)
		}
		bw.write(entry)
	}
	bw.line(") " + in.OutputType)
}

type fieldInfo struct {
	name       string
	goType     reflect.Type
	optional   bool
	defVal     string
	enumName   string
	scampiType string
}

func (f fieldInfo) resolveType() string {
	if f.scampiType != "" {
		return f.scampiType
	}
	if f.enumName != "" {
		return f.enumName
	}
	return goTypeToScampi(f.goType)
}

// Type mapping
// -----------------------------------------------------------------------------

func goTypeToScampi(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		return goTypeToScampi(t.Elem()) + "?"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Bool:
		return "bool"
	case reflect.Slice:
		return "list[" + goTypeToScampi(t.Elem()) + "]"
	case reflect.Map:
		return "map[" + goTypeToScampi(t.Key()) + ", " + goTypeToScampi(t.Elem()) + "]"
	case reflect.Interface:
		return "any"
	case reflect.Struct:
		return mapStructType(t)
	}
	return "any"
}

func mapStructType(t reflect.Type) string {
	full := t.PkgPath() + "." + t.Name()
	switch {
	case strings.HasSuffix(full, "spec.SourceRef"):
		return "Source"
	case strings.HasSuffix(full, "spec.PkgSourceRef"):
		return "PkgSource"
	case strings.HasSuffix(full, "target.Healthcheck"):
		return "Healthcheck"
	}
	return "any"
}

// Naming
// -----------------------------------------------------------------------------

func enumTypeName(stepKind, fieldName string) string {
	return capitalize(stepKind) + fieldName
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatDefault(val, enumName string) string {
	if enumName != "" {
		return enumName + "." + val
	}
	if val == "true" || val == "false" {
		return val
	}
	allDigits := true
	for _, c := range val {
		if (c < '0' || c > '9') && c != '-' {
			allDigits = false
			break
		}
	}
	if allDigits && val != "" {
		return val
	}
	return `"` + val + `"`
}

// Builder
// -----------------------------------------------------------------------------

type builder struct {
	w   io.Writer
	err error
}

func (b *builder) write(s string) {
	if b.err != nil {
		return
	}
	_, b.err = io.WriteString(b.w, s)
}

func (b *builder) line(s string) {
	b.write(s + "\n")
}
