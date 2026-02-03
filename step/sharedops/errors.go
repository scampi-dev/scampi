package sharedops

import (
	"fmt"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type UnknownUserError struct {
	User   string
	Source spec.SourceSpan
	Err    error
}

func (e UnknownUserError) Error() string {
	return fmt.Sprintf("unknown user %q: %v", e.User, e.Err)
}

func (e UnknownUserError) Unwrap() error {
	return e.Err
}

func (e UnknownUserError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.UnknownUserError",
		Text:   `unknown user "{{.User}}"`,
		Hint:   "create user before changing file owner",
		Data:   e,
		Source: &e.Source,
	}
}

func (UnknownUserError) Severity() signal.Severity { return signal.Error }
func (UnknownUserError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type UnknownGroupError struct {
	Group  string
	Source spec.SourceSpan
	Err    error
}

func (e UnknownGroupError) Error() string {
	return fmt.Sprintf("unknown group %q: %v", e.Group, e.Err)
}

func (e UnknownGroupError) Unwrap() error {
	return e.Err
}

func (e UnknownGroupError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.UnknownGroupError",
		Text:   `unknown group "{{.Group}}"`,
		Hint:   "create group before changing file owner",
		Data:   e,
		Source: &e.Source,
	}
}

func (UnknownGroupError) Severity() signal.Severity { return signal.Error }
func (UnknownGroupError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type PermissionDeniedError struct {
	Operation string
	Source    spec.SourceSpan
	Err       error
}

func (e PermissionDeniedError) Error() string {
	return fmt.Sprintf("%q: %v", e.Operation, e.Err)
}

func (e PermissionDeniedError) Unwrap() error {
	return e.Err
}

func (e PermissionDeniedError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.PermissionDeniedError",
		Text:   `permission denied for operation "{{.Operation}}"`,
		Hint:   "make sure you have the necessary permissions for this operation",
		Data:   e,
		Source: &e.Source,
	}
}

func (PermissionDeniedError) Severity() signal.Severity { return signal.Error }
func (PermissionDeniedError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type RelativePathError struct {
	Field  string
	Path   string
	Source spec.SourceSpan
}

func (e RelativePathError) Error() string {
	return fmt.Sprintf("relative path %q not allowed for %s", e.Path, e.Field)
}

func (e RelativePathError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.RelativePathError",
		Text:   `{{.Field}}: relative path not allowed`,
		Hint:   "target paths must be absolute (start with /)",
		Data:   e,
		Source: &e.Source,
	}
}

func (RelativePathError) Severity() signal.Severity { return signal.Error }
func (RelativePathError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
