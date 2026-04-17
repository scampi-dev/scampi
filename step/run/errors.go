// SPDX-License-Identifier: GPL-3.0-only

package run

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// ApplyError is emitted when the apply command fails.
type ApplyError struct {
	diagnostic.FatalError
	Cmd    string
	Stderr string
	Source spec.SourceSpan
}

func (e ApplyError) Error() string {
	return fmt.Sprintf("apply command failed: %s", e.Cmd)
}

func (e ApplyError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeApplyFailed,
		Text:   `apply command failed: {{.Cmd}}`,
		Hint:   `stderr: {{.Stderr}}`,
		Data:   e,
		Source: &e.Source,
	}
}

// PostApplyCheckError is emitted when the check command fails after apply.
type PostApplyCheckError struct {
	diagnostic.FatalError
	CheckCmd string
	ApplyCmd string
	Stderr   string
	Source   spec.SourceSpan
}

func (e PostApplyCheckError) Error() string {
	return fmt.Sprintf("post-apply check failed: %s", e.CheckCmd)
}

func (e PostApplyCheckError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodePostApplyCheckFailed,
		Text:   `post-apply check failed: {{.CheckCmd}}`,
		Hint:   `apply ran ({{.ApplyCmd}}) but check still fails: {{.Stderr}}`,
		Data:   e,
		Source: &e.Source,
	}
}

// CheckAlwaysConflictError is raised when both check and always are set.
type CheckAlwaysConflictError struct {
	diagnostic.FatalError
	Source spec.SourceSpan
}

func (e CheckAlwaysConflictError) Error() string {
	return "check and always are mutually exclusive"
}

func (e CheckAlwaysConflictError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeCheckAlwaysConflict,
		Text:   `check and always are mutually exclusive`,
		Hint:   `remove one: use check for idempotent commands, always for fire-and-forget`,
		Data:   e,
		Source: &e.Source,
	}
}

// MissingCheckOrAlwaysError is raised when neither check nor always is provided.
type MissingCheckOrAlwaysError struct {
	diagnostic.FatalError
	Source spec.SourceSpan
}

func (e MissingCheckOrAlwaysError) Error() string {
	return "run requires either check or always=True"
}

func (e MissingCheckOrAlwaysError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeMissingCheckOrAlways,
		Text:   `run requires either check or always=True`,
		Hint:   `add a check command for idempotency, or set always=True to run unconditionally`,
		Data:   e,
		Source: &e.Source,
	}
}
