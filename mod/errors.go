// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// ParseError
// -----------------------------------------------------------------------------

// ParseError is raised when scampi.mod cannot be parsed or contains invalid values.
type ParseError struct {
	diagnostic.FatalError
	Detail string
	Hint   string
	Source spec.SourceSpan
}

func (e ParseError) Error() string {
	if e.Source.StartLine > 0 {
		return fmt.Sprintf("%s:%d: %s", e.Source.Filename, e.Source.StartLine, e.Detail)
	}
	if e.Source.Filename != "" {
		return fmt.Sprintf("%s: %s", e.Source.Filename, e.Detail)
	}
	return e.Detail
}

func (e ParseError) EventTemplate() event.Template {
	return event.Template{
		ID:     "mod.ParseError",
		Text:   "{{.Detail}}",
		Hint:   "{{.Hint}}",
		Data:   e,
		Source: &e.Source,
	}
}

// ModuleNotFoundError
// -----------------------------------------------------------------------------

// ModuleNotFoundError is raised when a load path doesn't match any require entry.
type ModuleNotFoundError struct {
	diagnostic.FatalError
	LoadPath string
	Source   spec.SourceSpan
}

func (e *ModuleNotFoundError) Error() string {
	return fmt.Sprintf("module not found: %s", e.LoadPath)
}

func (e *ModuleNotFoundError) EventTemplate() event.Template {
	return event.Template{
		ID:     "mod.NotFound",
		Text:   "module not found: {{.LoadPath}}",
		Hint:   "add the module to scampi.mod and run: scampi mod tidy",
		Data:   e,
		Source: &e.Source,
	}
}

//nolint:unused // satisfies star.sourceSettable across package boundary
func (e *ModuleNotFoundError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// ModuleNotCachedError
// -----------------------------------------------------------------------------

// ModuleNotCachedError is raised when a module is in the require table but not downloaded.
type ModuleNotCachedError struct {
	diagnostic.FatalError
	ModPath string
	Version string
	Source  spec.SourceSpan
}

func (e *ModuleNotCachedError) Error() string {
	return fmt.Sprintf("module not cached: %s@%s", e.ModPath, e.Version)
}

func (e *ModuleNotCachedError) EventTemplate() event.Template {
	return event.Template{
		ID:     "mod.NotCached",
		Text:   "module not cached: {{.ModPath}}@{{.Version}}",
		Hint:   "run: scampi mod download",
		Data:   e,
		Source: &e.Source,
	}
}

//nolint:unused // satisfies star.sourceSettable across package boundary
func (e *ModuleNotCachedError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// ModuleNoEntryPointError
// -----------------------------------------------------------------------------

// ModuleNoEntryPointError is raised when a cached module has no loadable entry point file.
type ModuleNoEntryPointError struct {
	diagnostic.FatalError
	ModPath string
	Tried   []string
	Source  spec.SourceSpan
}

func (e *ModuleNoEntryPointError) Error() string {
	return fmt.Sprintf("module %s has no entry point", e.ModPath)
}

func (e *ModuleNoEntryPointError) EventTemplate() event.Template {
	return event.Template{
		ID:     "mod.NoEntryPoint",
		Text:   "module {{.ModPath}} has no entry point",
		Hint:   `tried: {{join ", " .Tried}}`,
		Data:   e,
		Source: &e.Source,
	}
}

//nolint:unused // satisfies star.sourceSettable across package boundary
func (e *ModuleNoEntryPointError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// ModInfo
// -----------------------------------------------------------------------------

// ModInfo is an informational diagnostic emitted by mod subcommands.
type ModInfo struct {
	diagnostic.Info
	Detail string
}

func (e *ModInfo) EventTemplate() event.Template {
	return event.Template{
		ID:   "mod.Info",
		Text: "{{.Detail}}",
		Data: e,
	}
}

// WriteError
// -----------------------------------------------------------------------------

// WriteError is raised when writing scampi.mod fails.
type WriteError struct {
	diagnostic.FatalError
	Detail string
	Hint   string
}

func (e *WriteError) Error() string { return e.Detail }

func (e *WriteError) EventTemplate() event.Template {
	return event.Template{
		ID:   "mod.WriteError",
		Text: "{{.Detail}}",
		Hint: "{{.Hint}}",
		Data: e,
	}
}

// InitError
// -----------------------------------------------------------------------------

// InitError is raised when scampi mod init fails.
type InitError struct {
	diagnostic.FatalError
	Detail string
	Hint   string
}

func (e *InitError) Error() string { return e.Detail }

func (e *InitError) EventTemplate() event.Template {
	return event.Template{
		ID:   "mod.InitError",
		Text: "{{.Detail}}",
		Hint: "{{.Hint}}",
		Data: e,
	}
}

// TidyError
// -----------------------------------------------------------------------------

// TidyError is raised when scampi mod tidy encounters an I/O or parse problem.
type TidyError struct {
	diagnostic.FatalError
	Detail string
	Hint   string
}

func (e *TidyError) Error() string { return e.Detail }

func (e *TidyError) EventTemplate() event.Template {
	return event.Template{
		ID:   "mod.TidyError",
		Text: "{{.Detail}}",
		Hint: "{{.Hint}}",
		Data: e,
	}
}

// SumError
// -----------------------------------------------------------------------------

// SumError is raised when I/O errors occur with hash computation or scampi.sum.
type SumError struct {
	diagnostic.FatalError
	Detail string
	Hint   string
}

func (e *SumError) Error() string { return e.Detail }

func (e *SumError) EventTemplate() event.Template {
	return event.Template{
		ID:   "mod.SumError",
		Text: "{{.Detail}}",
		Hint: "{{.Hint}}",
		Data: e,
	}
}

// FetchError
// -----------------------------------------------------------------------------

// FetchError is raised when cloning a module dependency fails.
type FetchError struct {
	diagnostic.FatalError
	ModPath string
	Version string
	Detail  string
	Hint    string
}

func (e *FetchError) Error() string {
	return fmt.Sprintf("fetch %s@%s: %s", e.ModPath, e.Version, e.Detail)
}

func (e *FetchError) EventTemplate() event.Template {
	return event.Template{
		ID:   "mod.FetchError",
		Text: "fetch {{.ModPath}}@{{.Version}}: {{.Detail}}",
		Hint: "{{.Hint}}",
		Data: e,
	}
}

// SumMismatchError
// -----------------------------------------------------------------------------

// SumMismatchError is raised when a cached module hash doesn't match the recorded sum.
type SumMismatchError struct {
	diagnostic.FatalError
	ModPath  string
	Version  string
	Expected string
	Actual   string
}

func (e *SumMismatchError) Error() string {
	return fmt.Sprintf("checksum mismatch for %s@%s", e.ModPath, e.Version)
}

func (e *SumMismatchError) EventTemplate() event.Template {
	return event.Template{
		ID:   "mod.SumMismatch",
		Text: "checksum mismatch for {{.ModPath}}@{{.Version}}",
		Hint: "the cached module may have been tampered with — run: scampi mod clean && scampi mod download",
		Data: e,
	}
}
