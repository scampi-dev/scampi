// SPDX-License-Identifier: GPL-3.0-only

// Package langstubs generates scampi-lang stub files from Go step
// config structs via reflection. It imports only stdlib — the caller
// provides concrete config struct pointers.
package langstubs

import (
	"errors"
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

// Options controls optional generator behavior.
type Options struct {
	AutoGenNotice bool
}

// Generate writes scampi-lang stub declarations for the named module.
func Generate(moduleName string, inputs []StubInput, opts Options, w io.Writer) error {
	if moduleName == "" {
		return errors.New("moduleName is required")
	}
	bw := &builder{w: w}

	bw.line("module " + moduleName)
	bw.line("")
	if opts.AutoGenNotice {
		bw.line("# Auto-generated from Go struct tags. Do not edit.")
		bw.line("")
	}

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
			doc:      stepTag,
			optional: f.Tag.Get("optional") == "true",
			defVal:   f.Tag.Get("default"),
		}
		if enumKey := findEnumKey(in.Enums, f.Name); enumKey != "" {
			fi.enumName = enumTypeName(in.Kind, enumKey)
		}
		fields = append(fields, fi)
	}

	fields = append(fields,
		fieldInfo{name: "desc", scampiType: "string?", optional: true},
		fieldInfo{name: "on_change", scampiType: "list[StepInstance]", defVal: "[]", rawDefault: true},
	)

	// Resolve types and compute column widths for alignment.
	type resolved struct {
		name    string
		typDef  string // "type" or "type = default"
		comment string
	}
	var rows []resolved
	maxName := 0
	maxTyp := 0
	for _, f := range fields {
		typStr := f.resolveType()
		if f.optional && !strings.HasSuffix(typStr, "?") {
			typStr += "?"
		}
		def := ""
		if f.defVal != "" {
			if f.rawDefault {
				def = " = " + f.defVal
			} else {
				def = " = " + formatDefault(f.defVal, f.enumName)
			}
		}
		r := resolved{
			name:    f.name,
			typDef:  typStr + def,
			comment: f.doc,
		}
		rows = append(rows, r)
		if len(r.name) > maxName {
			maxName = len(r.name)
		}
		if len(r.typDef) > maxTyp {
			maxTyp = len(r.typDef)
		}
	}

	bw.line("step " + in.Kind + "(")
	for i, r := range rows {
		comma := ","
		if i == len(rows)-1 {
			comma = ","
		}
		line := "  " + pad(r.name+":", maxName+1) + " " + pad(r.typDef+comma, maxTyp+1)
		if r.comment != "" {
			line += " # " + r.comment
		}
		bw.line(strings.TrimRight(line, " "))
	}
	bw.line(") " + in.OutputType)
}

type fieldInfo struct {
	name       string
	goType     reflect.Type
	doc        string // from step:"..." tag
	optional   bool
	defVal     string
	enumName   string
	scampiType string
	rawDefault bool
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
		return goTypeToScampi(t.Elem())
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
		return mapInterfaceType(t)
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
	case strings.HasSuffix(full, "rest.RequestConfig"):
		return "rest.request"
	case strings.HasSuffix(full, "rest.BodyConfig"):
		return "rest.body"
	case strings.HasSuffix(full, "rest.CheckConfig"):
		return "Check"
	case strings.HasSuffix(full, "rest.JQBinding"):
		return "Check"
	}
	return "any"
}

func mapInterfaceType(t reflect.Type) string {
	if t.PkgPath() == "" {
		return "any"
	}
	full := t.PkgPath() + "." + t.Name()
	switch {
	case strings.HasSuffix(full, "rest.BodyConfig"):
		return "rest.body"
	case strings.HasSuffix(full, "rest.CheckConfig"):
		return "Check"
	}
	return "any"
}

func pad(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}

// Naming
// -----------------------------------------------------------------------------

func findEnumKey(enums map[string][]string, fieldName string) string {
	if enums == nil {
		return ""
	}
	lower := strings.ToLower(fieldName)
	for k := range enums {
		if strings.ToLower(k) == lower {
			return k
		}
	}
	return ""
}

func enumTypeName(stepKind, fieldName string) string {
	// "container.instance" + "State" → "ContainerInstanceState"
	parts := strings.Split(stepKind, ".")
	var name string
	for _, p := range parts {
		name += capitalize(p)
	}
	return name + capitalize(fieldName)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func toSnake(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			// Insert underscore before uppercase if preceded by lowercase
			// or if this is the start of an acronym ending (e.g. GID → gid).
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
