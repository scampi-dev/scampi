// SPDX-License-Identifier: GPL-3.0-only

package group

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// GroupCreateError is emitted when creating a group fails.
type GroupCreateError struct {
	diagnostic.FatalError
	Name   string
	Err    error
	Source spec.SourceSpan
}

func (e GroupCreateError) Error() string {
	return fmt.Sprintf("failed to create group %q: %v", e.Name, e.Err)
}

func (e GroupCreateError) Unwrap() error { return e.Err }

func (e GroupCreateError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeCreateFailed,
		Text:   `failed to create group "{{.Name}}"`,
		Hint:   "check that the group name is valid and no conflicting group exists",
		Data:   e,
		Source: &e.Source,
	}
}

// GroupDeleteError is emitted when deleting a group fails.
type GroupDeleteError struct {
	diagnostic.FatalError
	Name   string
	Err    error
	Source spec.SourceSpan
}

func (e GroupDeleteError) Error() string {
	return fmt.Sprintf("failed to delete group %q: %v", e.Name, e.Err)
}

func (e GroupDeleteError) Unwrap() error { return e.Err }

func (e GroupDeleteError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeDeleteFailed,
		Text:   `failed to delete group "{{.Name}}"`,
		Hint:   "check that no users have this as their primary group",
		Data:   e,
		Source: &e.Source,
	}
}
