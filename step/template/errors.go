// SPDX-License-Identifier: GPL-3.0-only

package template

import (
	"fmt"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

type EnvKeyNotInValuesError struct {
	EnvVar string
	Key    string
	Source spec.SourceSpan
}

func (e EnvKeyNotInValuesError) Error() string {
	return fmt.Sprintf("env var %q maps to key %q which is not defined in values", e.EnvVar, e.Key)
}

func (e EnvKeyNotInValuesError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.template.EnvKeyNotInValues",
		Text:   `env var "{{.EnvVar}}" maps to key "{{.Key}}" which is not defined in values`,
		Hint:   "add the key to data.values or remove the env mapping",
		Help:   "all env mappings must reference keys that exist in data.values",
		Data:   e,
		Source: &e.Source,
	}
}

func (EnvKeyNotInValuesError) Severity() signal.Severity { return signal.Error }
func (EnvKeyNotInValuesError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type TemplateSourceMissingError struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e TemplateSourceMissingError) Error() string {
	return fmt.Sprintf("template source %q does not exist", e.Path)
}

func (e TemplateSourceMissingError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.template.SourceMissing",
		Text:   `template source "{{.Path}}" does not exist`,
		Hint:   "ensure the template file exists and is readable",
		Help:   "the template action cannot proceed without a readable source file",
		Data:   e,
		Source: &e.Source,
	}
}

func (TemplateSourceMissingError) Severity() signal.Severity { return signal.Error }
func (TemplateSourceMissingError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

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

func (e TemplateParseError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.template.Parse",
		Text:   "template parse error: {{.Err}}",
		Hint:   "check for unclosed braces, missing closing delimiters, or malformed variable names",
		Help:   "templates use Go text/template syntax: https://pkg.go.dev/text/template",
		Data:   e,
		Source: &e.Source,
	}
}

func (TemplateParseError) Severity() signal.Severity { return signal.Error }
func (TemplateParseError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

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

func (e TemplateExecError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.template.Exec",
		Text:   "template execution error: {{.Err}}",
		Hint:   "check that all referenced variables exist in data",
		Help:   "template execution failed, usually due to missing or mistyped variable names",
		Data:   e,
		Source: &e.Source,
	}
}

func (TemplateExecError) Severity() signal.Severity { return signal.Error }
func (TemplateExecError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type DestDirMissingError struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e DestDirMissingError) Error() string {
	return fmt.Sprintf("destination directory %q does not exist", e.Path)
}

func (e DestDirMissingError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.template.DestDirMissing",
		Text:   `destination directory "{{.Path}}" does not exist`,
		Hint:   "create the destination directory before running this action",
		Help:   "the template action does not create directories automatically",
		Data:   e,
		Source: &e.Source,
	}
}

func (DestDirMissingError) Severity() signal.Severity { return signal.Error }
func (DestDirMissingError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
func (e DestDirMissingError) DeferredPath() string    { return e.Path }

// MutuallyExclusiveError is raised when exactly one of a set of fields is
// required but zero or more than one were provided.
type MutuallyExclusiveError struct {
	Fields []string
	Got    []string
	Source spec.SourceSpan
}

func (e MutuallyExclusiveError) Error() string {
	return fmt.Sprintf("requires exactly one of %s", strings.Join(e.Fields, ", "))
}

func (e MutuallyExclusiveError) EventTemplate() event.Template {
	if len(e.Got) > 1 {
		return event.Template{
			ID:     "builtin.template.MutuallyExclusive",
			Text:   `requires exactly one of {{join ", " .Fields}}`,
			Hint:   "both were provided",
			Data:   e,
			Source: &e.Source,
		}
	}
	return event.Template{
		ID:     "builtin.template.MutuallyExclusive",
		Text:   `requires exactly one of {{join ", " .Fields}}`,
		Hint:   "neither was provided",
		Data:   e,
		Source: &e.Source,
	}
}

func (MutuallyExclusiveError) Severity() signal.Severity { return signal.Error }
func (MutuallyExclusiveError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
