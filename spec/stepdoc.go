package spec

// StepDoc contains documentation for a step type, extracted from CUE schema.
type StepDoc struct {
	Kind     string     // "copy", "symlink"
	Summary  string     // from @doc on struct
	Fields   []FieldDoc // derived from CUE
	Examples []string   // from Go interface (optional)
}

// FieldDoc contains documentation for a single field in a step.
type FieldDoc struct {
	Name     string // field label in CUE
	Type     string // "string", "int", "bool", "list", "struct"
	Required bool   // true if field: vs field?:
	Desc     string // from @doc attribute
	Default  string // from CUE default, empty if none
}
