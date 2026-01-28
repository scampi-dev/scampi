package engine

import (
	"fmt"
	"io/fs"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"godoit.dev/doit"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/spec"
)

// LoadStepDoc extracts documentation from the CUE schema for a step type.
// Panics on invariant violations (malformed embedded schemas).
func LoadStepDoc(kind string) spec.StepDoc {
	cueCtx := cuecontext.New()

	embFS, err := fs.Sub(doit.EmbeddedSchemaModule, "cue")
	if err != nil {
		panic(errs.BUG("embedded schema FS corrupted: %w", err))
	}

	loaderCfg := &load.Config{
		FS:  embFS,
		Dir: ".",
	}

	// Load the step kind schema
	// Invariant: kind was already validated against registry
	pkgPath := "godoit.dev/doit/kinds/" + kind
	instances := load.Instances([]string{pkgPath}, loaderCfg)
	if len(instances) == 0 {
		panic(errs.BUG("no CUE instance for registered kind %q", kind))
	}

	inst := instances[0]
	if inst.Err != nil {
		panic(errs.BUG("embedded schema for %q failed to load: %w", kind, inst.Err))
	}

	val := cueCtx.BuildInstance(inst)
	if err := val.Err(); err != nil {
		panic(errs.BUG("embedded schema for %q failed to build: %w", kind, err))
	}

	// Look up #Step definition
	stepDef := val.LookupPath(cue.ParsePath("#Step"))
	if !stepDef.Exists() {
		panic(errs.BUG("embedded schema for %q missing #Step definition", kind))
	}

	doc := spec.StepDoc{
		Kind:    kind,
		Summary: extractAttr(stepDef, "doc"),
		Fields:  extractFieldDocs(stepDef),
	}

	// Extract example if present
	if example := extractAttr(stepDef, "example"); example != "" {
		doc.Examples = []string{example}
	}

	return doc
}

// extractAttr extracts the first argument of a named attribute from a CUE value.
func extractAttr(v cue.Value, name string) string {
	for _, attr := range v.Attributes(cue.ValueAttr) {
		if attr.Name() == name && attr.NumArgs() > 0 {
			s, _ := attr.String(0)
			return s
		}
	}
	return ""
}

// extractFieldDocs iterates over struct fields and extracts their documentation.
func extractFieldDocs(stepDef cue.Value) []spec.FieldDoc {
	var fields []spec.FieldDoc

	// Iterate over all fields (including hidden ones with _)
	iter, err := stepDef.Fields(cue.All())
	if err != nil {
		return fields
	}

	for iter.Next() {
		sel := iter.Selector()
		name := sel.String()

		// Skip hidden fields (start with _)
		if strings.HasPrefix(name, "_") {
			continue
		}
		switch name {
		case "kind":
			continue
		}

		// Strip trailing "?" from optional field names
		name = strings.TrimSuffix(name, "?")

		fv := iter.Value()

		fieldDoc := spec.FieldDoc{
			Name:     name,
			Type:     cueKindToString(fv.IncompleteKind()),
			Required: !iter.IsOptional(),
			Desc:     extractAttr(fv, "doc"),
		}

		// Extract default value if present
		if def, ok := fv.Default(); ok {
			fieldDoc.Default = formatDefault(def)
		}

		fields = append(fields, fieldDoc)
	}

	return fields
}

// cueKindToString converts CUE kind to a human-readable type string.
func cueKindToString(k cue.Kind) string {
	switch k {
	case cue.StringKind:
		return "string"
	case cue.IntKind:
		return "int"
	case cue.FloatKind:
		return "float"
	case cue.BoolKind:
		return "bool"
	case cue.ListKind:
		return "list"
	case cue.StructKind:
		return "struct"
	case cue.BytesKind:
		return "bytes"
	case cue.NullKind:
		return "null"
	default:
		// For unions or other complex types
		return k.String()
	}
}

// formatDefault formats a default value for display.
func formatDefault(v cue.Value) string {
	if !v.IsConcrete() {
		return ""
	}

	switch v.Kind() {
	case cue.StringKind:
		s, _ := v.String()
		return `"` + s + `"`
	case cue.IntKind:
		i, _ := v.Int64()
		return fmt.Sprintf("%d", i)
	case cue.FloatKind:
		f, _ := v.Float64()
		return fmt.Sprintf("%g", f)
	case cue.BoolKind:
		b, _ := v.Bool()
		return fmt.Sprintf("%t", b)
	default:
		// For complex types, format the value
		bs, _ := v.MarshalJSON()
		return string(bs)
	}
}
