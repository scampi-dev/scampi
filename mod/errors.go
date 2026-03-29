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
