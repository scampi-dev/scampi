// SPDX-License-Identifier: GPL-3.0-only

package group

import (
	"fmt"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/spec"
)

// GroupCreateError is emitted when creating a group fails.
type GroupCreateError struct {
	Name string
	Err  error
	Span spec.Span
}

func (e GroupCreateError) Error() string {
	return fmt.Sprintf("failed to create group %q: %v", e.Name, e.Err)
}

func (e GroupCreateError) Unwrap() error { return e.Err }

func (e GroupCreateError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeCreateFailed,
			Text: `failed to create group "{{.Name}}"`,
			Hint: `verify "{{.Name}}" is a valid group name and no conflicting group exists on the target`,
			Help: `{{.Err}}`,
			Data: e,
			Span: &e.Span,
		},
	}
}

// GroupDeleteError is emitted when deleting a group fails.
type GroupDeleteError struct {
	Name string
	Err  error
	Span spec.Span
}

func (e GroupDeleteError) Error() string {
	return fmt.Sprintf("failed to delete group %q: %v", e.Name, e.Err)
}

func (e GroupDeleteError) Unwrap() error { return e.Err }

func (e GroupDeleteError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeDeleteFailed,
			Text: `failed to delete group "{{.Name}}"`,
			Hint: `confirm no users have "{{.Name}}" as their primary group ` +
				`(getent passwd | awk -F: '$4 == "<gid>"')`,
			Help: `{{.Err}}`,
			Data: e,
			Span: &e.Span,
		},
	}
}
