// SPDX-License-Identifier: GPL-3.0-only

package runset

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// ListFailedError is emitted when the list command fails (after init,
// if init was provided).
type ListFailedError struct {
	diagnostic.FatalError
	Cmd      string
	ExitCode int
	Stderr   string
	Source   spec.SourceSpan
}

func (e ListFailedError) Error() string {
	return fmt.Sprintf("run_set list failed: %s", e.Cmd)
}

func (e ListFailedError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeListFailed,
		Text: `run_set list command failed: {{.Cmd}} (exit {{.ExitCode}})`,
		Hint: `the list command must succeed and print one identifier per line; ` +
			`provide init to bootstrap when the set's container does not exist yet`,
		Help:   `{{.Stderr}}`,
		Data:   e,
		Source: &e.Source,
	}
}

// AddFailedError is emitted when the add command fails.
type AddFailedError struct {
	diagnostic.FatalError
	Cmd      string
	ExitCode int
	Stderr   string
	Source   spec.SourceSpan
}

func (e AddFailedError) Error() string {
	return fmt.Sprintf("run_set add failed: %s", e.Cmd)
}

func (e AddFailedError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeAddFailed,
		Text: `run_set add command failed: {{.Cmd}} (exit {{.ExitCode}})`,
		Hint: `the add command must succeed for the missing items; ` +
			`check the rendered command above and the stderr below`,
		Help:   `{{.Stderr}}`,
		Data:   e,
		Source: &e.Source,
	}
}

// RemoveFailedError is emitted when the remove command fails.
type RemoveFailedError struct {
	diagnostic.FatalError
	Cmd      string
	ExitCode int
	Stderr   string
	Source   spec.SourceSpan
}

func (e RemoveFailedError) Error() string {
	return fmt.Sprintf("run_set remove failed: %s", e.Cmd)
}

func (e RemoveFailedError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeRemoveFailed,
		Text: `run_set remove command failed: {{.Cmd}} (exit {{.ExitCode}})`,
		Hint: `the remove command must succeed for the orphan items; ` +
			`check the rendered command above and the stderr below`,
		Help:   `{{.Stderr}}`,
		Data:   e,
		Source: &e.Source,
	}
}

// InitFailedError is emitted when the init bootstrap command fails.
type InitFailedError struct {
	diagnostic.FatalError
	Cmd      string
	ExitCode int
	Stderr   string
	Source   spec.SourceSpan
}

func (e InitFailedError) Error() string {
	return fmt.Sprintf("run_set init failed: %s", e.Cmd)
}

func (e InitFailedError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeInitFailed,
		Text:   `run_set init command failed: {{.Cmd}} (exit {{.ExitCode}})`,
		Hint:   `init runs only when list returns non-zero; verify init creates the container that list queries`,
		Help:   `{{.Stderr}}`,
		Data:   e,
		Source: &e.Source,
	}
}

// MissingTemplateError is raised at plan time when the template in
// add or remove has no recognised placeholder.
type MissingTemplateError struct {
	diagnostic.FatalError
	Field  string // "add" or "remove"
	Cmd    string
	Source spec.SourceSpan
}

func (e MissingTemplateError) Error() string {
	return fmt.Sprintf("run_set %s template has no placeholder", e.Field)
}

func (e MissingTemplateError) EventTemplate() event.Template {
	return event.Template{
		ID: CodeMissingTemplate,
		Text: `run_set {{.Field}} template has no item placeholder: ` +
			`{{.Cmd}}`,
		Hint: `use {{"{{ item }}"}} for per-item invocations, ` +
			`{{"{{ items }}"}} for space-separated batch, ` +
			`or {{"{{ items_csv }}"}} for comma-separated batch`,
		Data:   e,
		Source: &e.Source,
	}
}

// InvalidTemplateError flags a template that mixes per-item with
// batch placeholders.
type InvalidTemplateError struct {
	diagnostic.FatalError
	Field  string
	Cmd    string
	Source spec.SourceSpan
}

func (e InvalidTemplateError) Error() string {
	return fmt.Sprintf("run_set %s template mixes per-item and batch placeholders", e.Field)
}

func (e InvalidTemplateError) EventTemplate() event.Template {
	return event.Template{
		ID: CodeInvalidTemplate,
		Text: `run_set {{.Field}} template mixes {{"{{ item }}"}} with ` +
			`{{"{{ items }}"}} or {{"{{ items_csv }}"}}: {{.Cmd}}`,
		Hint:   `pick one — per-item runs the command once per element, batch runs it once with all elements`,
		Data:   e,
		Source: &e.Source,
	}
}

// NothingToDeclareError is raised at plan time when neither add
// nor remove is provided — the step would be a noop forever.
type NothingToDeclareError struct {
	diagnostic.FatalError
	Source spec.SourceSpan
}

func (e NothingToDeclareError) Error() string {
	return "run_set requires at least one of add or remove"
}

func (e NothingToDeclareError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeNothingToDeclare,
		Text:   `run_set requires at least one of add or remove`,
		Hint:   `add for desired-not-live items, remove for live-not-desired (orphan) items; both are typical`,
		Data:   e,
		Source: &e.Source,
	}
}
