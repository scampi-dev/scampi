// SPDX-License-Identifier: GPL-3.0-only

package spec

// Span locates a region of scampi source, used to anchor diagnostics.
type Span struct {
	Filename  string
	StartLine int
	EndLine   int
	StartCol  int
	EndCol    int
}

// FieldSpan carries the source spans of a config field's name and value.
type FieldSpan struct {
	Field Span
	Value Span
}
