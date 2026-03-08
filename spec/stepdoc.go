// SPDX-License-Identifier: GPL-3.0-only

package spec

import (
	"fmt"
	"strings"
)

// StepDoc contains documentation for a step type.
type StepDoc struct {
	Kind    string
	Summary string
	Fields  []FieldDoc
}

// FieldDoc contains documentation for a single field in a step.
type FieldDoc struct {
	Name     string
	Type     string // "string", "int", "bool", "list", "struct"
	Required bool
	Desc     string
	Default  string   // display string, empty if none
	Examples []string // example values (from pipe-delimited example tag)
}

// Examples builds code snippets from field metadata (example/default tags).
func (d StepDoc) Examples() []string {
	var b strings.Builder
	b.WriteString(d.Kind + "(\n")
	for _, f := range d.Fields {
		val, ok := exampleValue(f)
		if !ok {
			continue
		}
		b.WriteString("    " + f.Name + "=" + val + ",\n")
	}
	b.WriteString(")")
	return []string{b.String()}
}

func exampleValue(f FieldDoc) (string, bool) {
	var raw string
	if len(f.Examples) > 0 {
		raw = f.Examples[0]
	}
	if raw == "" {
		raw = strings.Trim(f.Default, `"`)
	}

	switch f.Type {
	case "bool":
		if raw == "true" {
			return "True", true
		}
		return "False", true
	case "int":
		if raw != "" {
			return raw, true
		}
		return "0", true
	case "list":
		if raw != "" {
			return raw, true
		}
		return "", false
	case "string":
		if raw != "" {
			return fmt.Sprintf("%q", raw), true
		}
		return "", false
	default:
		if raw != "" {
			return fmt.Sprintf("%q", raw), true
		}
		return "", false
	}
}
