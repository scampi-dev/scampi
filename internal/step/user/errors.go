// SPDX-License-Identifier: GPL-3.0-only

package user

import (
	"fmt"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/spec"
)

// UserCreateError is emitted when creating a user fails.
type UserCreateError struct {
	Name   string
	Err    error
	Source spec.SourceSpan
}

func (e UserCreateError) Error() string {
	return fmt.Sprintf("failed to create user %q: %v", e.Name, e.Err)
}

func (e UserCreateError) Unwrap() error { return e.Err }

func (e UserCreateError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeCreateFailed,
			Text:   `failed to create user "{{.Name}}"`,
			Hint:   `verify "{{.Name}}" is a valid username and no conflicting user/uid already exists`,
			Help:   `{{.Err}}`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

// UserModifyError is emitted when modifying a user fails.
type UserModifyError struct {
	Name   string
	Err    error
	Source spec.SourceSpan
}

func (e UserModifyError) Error() string {
	return fmt.Sprintf("failed to modify user %q: %v", e.Name, e.Err)
}

func (e UserModifyError) Unwrap() error { return e.Err }

func (e UserModifyError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeModifyFailed,
			Text:   `failed to modify user "{{.Name}}"`,
			Hint:   `confirm user "{{.Name}}" exists on the target and the requested groups/shell/home are valid`,
			Help:   `{{.Err}}`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

// UserDeleteError is emitted when deleting a user fails.
type UserDeleteError struct {
	Name   string
	Err    error
	Source spec.SourceSpan
}

func (e UserDeleteError) Error() string {
	return fmt.Sprintf("failed to delete user %q: %v", e.Name, e.Err)
}

func (e UserDeleteError) Unwrap() error { return e.Err }

func (e UserDeleteError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeDeleteFailed,
			Text:   `failed to delete user "{{.Name}}"`,
			Hint:   `confirm no running processes belong to "{{.Name}}"; run: ps -u {{.Name}}`,
			Help:   `{{.Err}}`,
			Data:   e,
			Source: &e.Source,
		},
	}
}
