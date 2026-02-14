// SPDX-License-Identifier: GPL-3.0-only

package spec

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
	Default  string // display string, empty if none
	Example  string // example value, empty if none
}
