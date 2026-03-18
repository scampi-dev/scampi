// SPDX-License-Identifier: GPL-3.0-only

package spec

// SourceRefKind identifies how a source was provided.
type SourceRefKind uint8

const (
	SourceLocal SourceRefKind = iota
	SourceInline
	SourceRemote
)

// SourceRef is a resolved reference to source content. For local and inline
// sources, Path is set at eval time. For remote sources, Path is the
// deterministic cache path where the download op will write the file.
type SourceRef struct {
	Kind     SourceRefKind
	Path     string // cache path (set for all kinds after eval)
	Content  string // SourceInline only: original content (for display/diagnostics)
	URL      string // SourceRemote only: download URL
	Checksum string // SourceRemote only: expected checksum ("algo:hex")
}

func (r SourceRef) NeedsResolution() bool {
	return r.Kind == SourceRemote
}

func (r SourceRef) IsZero() bool {
	return r.Path == "" && r.Content == "" && r.URL == ""
}

func (r SourceRef) DisplayPath() string {
	switch r.Kind {
	case SourceInline:
		return "(inline)"
	case SourceRemote:
		return r.URL
	default:
		return r.Path
	}
}
