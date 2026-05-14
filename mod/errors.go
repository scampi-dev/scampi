// SPDX-License-Identifier: GPL-3.0-only

package mod

import (
	"fmt"
	"strings"

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
		ID:     CodeParseError,
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
		ID:     CodeNotFound,
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
		ID:     CodeNotCached,
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
		ID:     CodeNoEntryPoint,
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
		ID:   CodeInfo,
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
	Source spec.SourceSpan
}

func (e *WriteError) Error() string { return e.Detail }

func (e *WriteError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeWriteError,
		Text:   "{{.Detail}}",
		Hint:   "{{.Hint}}",
		Data:   e,
		Source: &e.Source,
	}
}

// InitError
// -----------------------------------------------------------------------------

// InitError is raised when scampi mod init fails.
type InitError struct {
	diagnostic.FatalError
	Detail string
	Hint   string
	Source spec.SourceSpan
}

func (e *InitError) Error() string { return e.Detail }

func (e *InitError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeInitError,
		Text:   "{{.Detail}}",
		Hint:   "{{.Hint}}",
		Data:   e,
		Source: &e.Source,
	}
}

// InitStatError
// -----------------------------------------------------------------------------

// InitStatError is raised when scampi mod init cannot stat the target
// scampi.mod path (e.g. permission denied on the parent directory).
//
// Uses structured fields + a literal-template EventTemplate so dynamic
// content cannot leak through {{.Detail}}-style interpolation.
type InitStatError struct {
	diagnostic.FatalError
	Path  string
	Cause error
}

func (e *InitStatError) Error() string {
	return fmt.Sprintf("could not stat %q: %v", e.Path, e.Cause)
}

func (e *InitStatError) Unwrap() error { return e.Cause }

func (e *InitStatError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeInitStatError,
		Text: `could not stat "{{.Path}}"`,
		Hint: "check directory permissions",
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
	Source spec.SourceSpan
}

func (e *TidyError) Error() string { return e.Detail }

func (e *TidyError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeTidyError,
		Text:   "{{.Detail}}",
		Hint:   "{{.Hint}}",
		Data:   e,
		Source: &e.Source,
	}
}

// SumError
// -----------------------------------------------------------------------------

// SumError is raised when I/O errors occur with hash computation or scampi.sum.
type SumError struct {
	diagnostic.FatalError
	Detail string
	Hint   string
	Source spec.SourceSpan
}

func (e *SumError) Error() string { return e.Detail }

func (e *SumError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeSumError,
		Text:   "{{.Detail}}",
		Hint:   "{{.Hint}}",
		Data:   e,
		Source: &e.Source,
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
	Source  spec.SourceSpan
}

func (e *FetchError) Error() string {
	return fmt.Sprintf("fetch %s@%s: %s", e.ModPath, e.Version, e.Detail)
}

func (e *FetchError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeFetchError,
		Text:   "fetch {{.ModPath}}@{{.Version}}: {{.Detail}}",
		Hint:   "{{.Hint}}",
		Data:   e,
		Source: &e.Source,
	}
}

// NotAModuleError
// -----------------------------------------------------------------------------

// NotAModuleError is raised when a fetched repo has no .scampi entry point.
type NotAModuleError struct {
	diagnostic.FatalError
	ModPath string
	Version string
	Source  spec.SourceSpan
}

func (e *NotAModuleError) Error() string {
	return fmt.Sprintf("%s@%s is not a scampi module", e.ModPath, e.Version)
}

func (e *NotAModuleError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeNotAModule,
		Text:   "{{.ModPath}}@{{.Version}} is not a scampi module",
		Hint:   "a module must contain _index.scampi or <name>.scampi at its root",
		Data:   e,
		Source: &e.Source,
	}
}

// AddError
// -----------------------------------------------------------------------------

// AddError is raised when scampi mod add fails.
type AddError struct {
	diagnostic.FatalError
	Detail string
	Hint   string
	Source spec.SourceSpan
}

func (e *AddError) Error() string { return e.Detail }

func (e *AddError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeAddError,
		Text:   "{{.Detail}}",
		Hint:   "{{.Hint}}",
		Data:   e,
		Source: &e.Source,
	}
}

// NoStableVersionError
// -----------------------------------------------------------------------------

// NoStableVersionError is raised when no stable semver tags are found for a module.
type NoStableVersionError struct {
	diagnostic.FatalError
	ModPath string
	Source  spec.SourceSpan
}

func (e *NoStableVersionError) Error() string {
	return "no stable version found for " + e.ModPath
}

func (e *NoStableVersionError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeNoStableVersion,
		Text:   "no stable version found for {{.ModPath}}",
		Hint:   "specify a version explicitly: scampi mod add {{.ModPath}}@v1.0.0-alpha.1",
		Data:   e,
		Source: &e.Source,
	}
}

// CycleError
// -----------------------------------------------------------------------------

// CycleError is raised when transitive dependency resolution detects a cycle.
type CycleError struct {
	diagnostic.FatalError
	Chain  []string
	Source spec.SourceSpan
}

func (e *CycleError) Error() string {
	return "dependency cycle detected: " + strings.Join(e.Chain, " -> ")
}

func (e *CycleError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeCycleError,
		Text:   "dependency cycle detected",
		Hint:   `{{join " -> " .Chain}}`,
		Data:   e,
		Source: &e.Source,
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
	Source   spec.SourceSpan
}

func (e *SumMismatchError) Error() string {
	return fmt.Sprintf("checksum mismatch for %s@%s", e.ModPath, e.Version)
}

func (e *SumMismatchError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeSumMismatch,
		Text:   "checksum mismatch for {{.ModPath}}@{{.Version}}",
		Hint:   "the cached module may have been tampered with — run: scampi mod clean && scampi mod download",
		Data:   e,
		Source: &e.Source,
	}
}

// DirectPinConflictError
// -----------------------------------------------------------------------------

// DirectPinConflictError is raised when a transitive dependency requires a
// higher version of a module than the project directly pins. MVS would
// silently upgrade the pin; this error makes the conflict explicit so the
// user can either bump the direct require or fix the transitive demand.
type DirectPinConflictError struct {
	diagnostic.FatalError
	ModPath           string
	DirectVersion     string
	TransitiveVersion string
	DemandedBy        string // the dependency whose transitive require triggered the conflict
	Source            spec.SourceSpan
}

func (e *DirectPinConflictError) Error() string {
	return fmt.Sprintf(
		"%s pinned to %s but %s requires %s",
		e.ModPath, e.DirectVersion, e.DemandedBy, e.TransitiveVersion,
	)
}

func (e *DirectPinConflictError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeDirectPin,
		Text: "{{.ModPath}} pinned to {{.DirectVersion}} but {{.DemandedBy}} requires {{.TransitiveVersion}}",
		Hint: "bump the direct require in scampi.mod, " +
			"or use an older {{.DemandedBy}} that doesn't demand the upgrade",
		Data:   e,
		Source: &e.Source,
	}
}
