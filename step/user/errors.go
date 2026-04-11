// SPDX-License-Identifier: GPL-3.0-only

package user

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// UserCreateError is emitted when creating a user fails.
type UserCreateError struct {
	diagnostic.FatalError
	Name   string
	Err    error
	Source spec.SourceSpan
}

func (e UserCreateError) Error() string {
	return fmt.Sprintf("failed to create user %q: %v", e.Name, e.Err)
}

func (e UserCreateError) Unwrap() error { return e.Err }

func (e UserCreateError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.user.CreateFailed",
		Text:   `failed to create user "{{.Name}}"`,
		Hint:   "check that the username is valid and no conflicting user exists",
		Data:   e,
		Source: &e.Source,
	}
}

// UserModifyError is emitted when modifying a user fails.
type UserModifyError struct {
	diagnostic.FatalError
	Name   string
	Err    error
	Source spec.SourceSpan
}

func (e UserModifyError) Error() string {
	return fmt.Sprintf("failed to modify user %q: %v", e.Name, e.Err)
}

func (e UserModifyError) Unwrap() error { return e.Err }

func (e UserModifyError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.user.ModifyFailed",
		Text:   `failed to modify user "{{.Name}}"`,
		Hint:   "check that the user exists and the target values are valid",
		Data:   e,
		Source: &e.Source,
	}
}

// UserDeleteError is emitted when deleting a user fails.
type UserDeleteError struct {
	diagnostic.FatalError
	Name   string
	Err    error
	Source spec.SourceSpan
}

func (e UserDeleteError) Error() string {
	return fmt.Sprintf("failed to delete user %q: %v", e.Name, e.Err)
}

func (e UserDeleteError) Unwrap() error { return e.Err }

func (e UserDeleteError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.user.DeleteFailed",
		Text:   `failed to delete user "{{.Name}}"`,
		Hint:   "check that no running processes belong to this user",
		Data:   e,
		Source: &e.Source,
	}
}
