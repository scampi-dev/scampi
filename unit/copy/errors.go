package copy

import (
	"fmt"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type CopySourceMissing struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e CopySourceMissing) Error() string {
	return fmt.Sprintf("source file %q does not exist", e.Path)
}

func (e CopySourceMissing) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
}

func (e CopySourceMissing) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.copy.SourceMissing",
		Text:   `source file "{{.Path}}" does not exist`,
		Hint:   "ensure the source file exists and is readable",
		Help:   "the copy action cannot proceed without a readable source file",
		Data:   e,
		Source: &e.Source,
	}
}

func (CopySourceMissing) Severity() signal.Severity {
	return signal.Error
}

type CopyDestDirMissing struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e CopyDestDirMissing) Error() string {
	return fmt.Sprintf("destination directory %q does not exist", e.Path)
}

func (e CopyDestDirMissing) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
}

func (e CopyDestDirMissing) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.copy.DestDirMissing",
		Text:   `destination directory "{{.Path}}" does not exist`,
		Hint:   "create the destination directory before running this action",
		Help:   "the copy action does not create directories automatically",
		Data:   e,
		Source: &e.Source,
	}
}

func (CopyDestDirMissing) Severity() signal.Severity {
	return signal.Error
}

type UserNotFound struct {
	User   string
	Source spec.SourceSpan
}

func (e UserNotFound) Error() string {
	return fmt.Sprintf("user %q does not exist on target", e.User)
}

func (e UserNotFound) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
}

func (e UserNotFound) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.copy.UserNotFound",
		Text:   `user "{{.User}}" does not exist on target`,
		Hint:   "check the user name or create the user on the target system",
		Help:   "ownership cannot be set to a user that does not exist",
		Data:   e,
		Source: &e.Source,
	}
}

func (UserNotFound) Severity() signal.Severity {
	return signal.Error
}

type GroupNotFound struct {
	Group  string
	Source spec.SourceSpan
}

func (e GroupNotFound) Error() string {
	return fmt.Sprintf("group %q does not exist on target", e.Group)
}

func (e GroupNotFound) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
}

func (e GroupNotFound) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.copy.GroupNotFound",
		Text:   `group "{{.Group}}" does not exist on target`,
		Hint:   "check the group name or create the group on the target system",
		Help:   "ownership cannot be set to a group that does not exist",
		Data:   e,
		Source: &e.Source,
	}
}

func (GroupNotFound) Severity() signal.Severity {
	return signal.Error
}

type OwnerReadError struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e OwnerReadError) Error() string {
	return fmt.Sprintf("cannot read ownership of %q: %v", e.Path, e.Err)
}

func (e OwnerReadError) Unwrap() error {
	return e.Err
}

func (e OwnerReadError) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
}

func (e OwnerReadError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.copy.OwnerReadError",
		Text:   `cannot read ownership of "{{.Path}}"`,
		Hint:   "check file permissions and ensure the path is accessible",
		Data:   e,
		Source: &e.Source,
	}
}

func (OwnerReadError) Severity() signal.Severity {
	return signal.Error
}

type ModeReadError struct {
	Path   string
	Source spec.SourceSpan
	Err    error
}

func (e ModeReadError) Error() string {
	return fmt.Sprintf("cannot read mode of %q: %v", e.Path, e.Err)
}

func (e ModeReadError) Unwrap() error {
	return e.Err
}

func (e ModeReadError) Diagnostics(subject event.Subject) []event.Event {
	return []event.Event{
		diagnostic.DiagnosticRaised(subject, e),
	}
}

func (e ModeReadError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.copy.ModeReadError",
		Text:   `cannot read mode of "{{.Path}}"`,
		Hint:   "check file permissions and ensure the path is accessible",
		Data:   e,
		Source: &e.Source,
	}
}

func (ModeReadError) Severity() signal.Severity {
	return signal.Error
}
