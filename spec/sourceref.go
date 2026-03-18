// SPDX-License-Identifier: GPL-3.0-only

package spec

// SourceRefKind identifies how a source was provided.
type SourceRefKind uint8

const (
	SourceLocal SourceRefKind = iota
	SourceInline
)

// SourceRef is a resolved reference to source content. By the time Plan()
// runs, Path is always a usable file path — inline content has already been
// written to the cache.
type SourceRef struct {
	Kind    SourceRefKind
	Path    string // always set after resolution
	Content string // SourceInline only: original content (for display/diagnostics)
}

func (r SourceRef) IsZero() bool {
	return r.Path == "" && r.Content == ""
}

func (r SourceRef) DisplayPath() string {
	if r.Kind == SourceInline {
		return "(inline)"
	}
	return r.Path
}
