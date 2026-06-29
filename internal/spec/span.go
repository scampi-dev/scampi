// SPDX-License-Identifier: GPL-3.0-only

package spec

// SourceSpan locates a region of scampi source, used to anchor diagnostics.
type SourceSpan struct {
	Filename  string
	StartLine int
	EndLine   int
	StartCol  int
	EndCol    int
}

// FieldSpan carries the source spans of a config field's name and value.
type FieldSpan struct {
	Field SourceSpan
	Value SourceSpan
}
