// SPDX-License-Identifier: GPL-3.0-only

package template

import (
	"fmt"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/spec"
)

type EnvKeyNotInValuesError struct {
	EnvVar string
	Key    string
	Source spec.SourceSpan
}

func (e EnvKeyNotInValuesError) Error() string {
	return fmt.Sprintf("env var %q maps to key %q which is not defined in values", e.EnvVar, e.Key)
}

func (e EnvKeyNotInValuesError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeEnvKeyNotInValues,
			Text:   `env var "{{.EnvVar}}" maps to key "{{.Key}}" which is not defined in values`,
			Hint:   "add the key to data.values or remove the env mapping",
			Help:   "all env mappings must reference keys that exist in data.values",
			Data:   e,
			Source: &e.Source,
		},
	}
}

type TemplateSourceMissingError struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e TemplateSourceMissingError) Error() string {
	return fmt.Sprintf("template source %q does not exist", e.Path)
}

func (e TemplateSourceMissingError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeSourceMissing,
			Text:   `template source "{{.Path}}" does not exist`,
			Hint:   `add "{{.Path}}" to the source tree, or fix the path passed to source_local(...)`,
			Help:   "the template action cannot proceed without a readable source file",
			Data:   e,
			Source: &e.Source,
		},
	}
}

type TemplateParseError struct {
	Err    error
	Source spec.SourceSpan
}

func (e TemplateParseError) Error() string {
	return fmt.Sprintf("template parse error: %v", e.Err)
}

func (e TemplateParseError) Unwrap() error {
	return e.Err
}

func (e TemplateParseError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeParse,
			Text:   "template parse error: {{.Err}}",
			Hint:   "check for unclosed braces, missing closing delimiters, or malformed variable names",
			Help:   "templates use Go text/template syntax: https://pkg.go.dev/text/template",
			Data:   e,
			Source: &e.Source,
		},
	}
}

type TemplateExecError struct {
	Err    error
	Source spec.SourceSpan
}

func (e TemplateExecError) Error() string {
	return fmt.Sprintf("template execution error: %v", e.Err)
}

func (e TemplateExecError) Unwrap() error {
	return e.Err
}

func (e TemplateExecError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeExec,
			Text:   "template execution error: {{.Err}}",
			Hint:   "check that all referenced variables exist in data",
			Help:   "template execution failed, usually due to missing or mistyped variable names",
			Data:   e,
			Source: &e.Source,
		},
	}
}

type DestDirMissingError struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e DestDirMissingError) Error() string {
	return fmt.Sprintf("destination directory %q does not exist", e.Path)
}

func (e DestDirMissingError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeDestDirMissing,
			Text:   `destination directory "{{.Path}}" does not exist`,
			Hint:   "create the destination directory before running this action",
			Help:   "the template action does not create directories automatically",
			Data:   e,
			Source: &e.Source,
		},
	}
}

func (e DestDirMissingError) DeferredResource() spec.Resource {
	return spec.PathResource(e.Path)
}
