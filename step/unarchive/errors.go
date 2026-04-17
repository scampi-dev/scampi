// SPDX-License-Identifier: GPL-3.0-only

package unarchive

import (
	"fmt"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// UnsupportedArchiveError is raised at plan time for unknown archive extensions.
type UnsupportedArchiveError struct {
	diagnostic.FatalError
	Path   string
	Source spec.SourceSpan
}

func (e UnsupportedArchiveError) Error() string {
	return fmt.Sprintf("unsupported archive format: %q", e.Path)
}

func (e UnsupportedArchiveError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeUnsupportedArchive,
		Text:   `unsupported archive format: "{{.Path}}"`,
		Hint:   "supported: .tar.gz, .tgz, .tar.bz2, .tbz2, .tar.xz, .txz, .tar.zst, .tzst, .tar, .zip",
		Data:   e,
		Source: &e.Source,
	}
}

// ArchiveNotFoundError is raised when the source archive does not exist.
type ArchiveNotFoundError struct {
	diagnostic.FatalError
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e ArchiveNotFoundError) Error() string {
	return fmt.Sprintf("source archive %q does not exist", e.Path)
}

func (e ArchiveNotFoundError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeArchiveNotFound,
		Text:   `source archive "{{.Path}}" does not exist`,
		Hint:   "ensure the archive file exists and is readable",
		Data:   e,
		Source: &e.Source,
	}
}

// ExtractionError is raised when the extract command fails.
type ExtractionError struct {
	diagnostic.FatalError
	Cmd    string
	Stderr string
	Advice string
	Source spec.SourceSpan
}

func (e ExtractionError) Error() string {
	return fmt.Sprintf("extraction failed: %s: %s", e.Cmd, e.Stderr)
}

func (e ExtractionError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeExtractionFailed,
		Text:   `extraction failed: {{.Cmd}}`,
		Hint:   "{{.Advice}}",
		Help:   "{{.Stderr}}",
		Data:   e,
		Source: &e.Source,
	}
}

// ArchiveReadError is raised when reading archive contents fails during extraction.
type ArchiveReadError struct {
	diagnostic.FatalError
	Format string
	Entry  string
	Err    error
}

func (e ArchiveReadError) Error() string {
	if e.Entry != "" {
		return fmt.Sprintf("reading %s entry %q: %v", e.Format, e.Entry, e.Err)
	}
	return fmt.Sprintf("reading %s: %v", e.Format, e.Err)
}

func (e ArchiveReadError) Unwrap() error { return e.Err }

func (e ArchiveReadError) EventTemplate() event.Template {
	if e.Entry != "" {
		return event.Template{
			ID:   CodeArchiveReadEntry,
			Text: `reading {{.Format}} entry "{{.Entry}}" failed`,
			Hint: "the archive may be corrupt or truncated",
			Data: e,
		}
	}
	return event.Template{
		ID:   CodeArchiveRead,
		Text: "reading {{.Format}} failed",
		Hint: "the archive may be corrupt or truncated",
		Data: e,
	}
}

func extractionAdvice(stderr string) string {
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "cannot open") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "operation not permitted") {
		return "target contains files not writable by the connecting user — check ownership with ls -la"
	}
	return "check that the archive is not corrupt and required tools are installed"
}

// PartialOwnershipError is raised at plan time when owner is set without group
// or vice versa.
type PartialOwnershipError struct {
	diagnostic.FatalError
	Set     string
	Missing string
	Source  spec.SourceSpan
}

func (e PartialOwnershipError) Error() string {
	return fmt.Sprintf("%s is set but %s is empty — set both or neither", e.Set, e.Missing)
}

func (e PartialOwnershipError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodePartialOwnership,
		Text:   `{{.Set}} is set but {{.Missing}} is empty`,
		Hint:   `add {{.Missing}}="<value>" or remove {{.Set}}`,
		Data:   e,
		Source: &e.Source,
	}
}
