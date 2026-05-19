// SPDX-License-Identifier: GPL-3.0-only

package sharedop

import (
	"errors"
	"fmt"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
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

func (e UnknownUserError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeUnknownUser,
			Text:   `unknown user "{{.User}}"`,
			Hint:   `create user "{{.User}}" with useradd or adduser before setting file owner`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

func (e UnknownUserError) DeferredResource() spec.Resource {
	return spec.UserResource(e.User)
}

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

func (e UnknownGroupError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeUnknownGroup,
			Text:   `unknown group "{{.Group}}"`,
			Hint:   `create group "{{.Group}}" with groupadd or addgroup before setting file owner`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

func (e UnknownGroupError) DeferredResource() spec.Resource {
	return spec.GroupResource(e.Group)
}

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

func (e PermissionDeniedError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodePermissionDenied,
			Text:   `permission denied for operation "{{.Operation}}"`,
			Hint:   "run as root, or configure passwordless sudo/doas for the target user",
			Data:   e,
			Source: &e.Source,
		},
	}
}

// EscalationFailedError wraps a target.EscalationError with diagnostic metadata.
type EscalationFailedError struct {
	target.EscalationError
}

func (e EscalationFailedError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeEscalationFailed,
			Text: `{{.Tool}} {{.Op}} {{.Path}}: exit {{.ExitCode}}`,
			Hint: "the target user may lack passwordless sudo/doas",
			Help: "{{.Stderr}}",
			Data: e.EscalationError,
		},
	}
}

// EscalationMissingError wraps a target.NoEscalationError with diagnostic metadata.
type EscalationMissingError struct {
	target.NoEscalationError
}

func (e EscalationMissingError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeEscalationMissing,
			Text: `{{.Op}} {{.Path}}: no escalation tool found`,
			Hint: "install sudo or doas on the target, or run as root",
			Data: e.NoEscalationError,
		},
	}
}

// StagingFailedError wraps a target.StagingError with diagnostic metadata.
type StagingFailedError struct {
	target.StagingError
}

func (e StagingFailedError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeStagingFailed,
			Text: `failed to stage temp file for "{{.Path}}"`,
			Hint: "ensure /tmp is writable on the target",
			Data: e.StagingError,
		},
	}
}

// DiagnoseTargetError wraps known target-layer errors in diagnostic types.
// Returns the original error unchanged if not a recognized target error.
func DiagnoseTargetError(err error) error {
	var noEsc target.NoEscalationError
	if errors.As(err, &noEsc) {
		return EscalationMissingError{NoEscalationError: noEsc}
	}
	var escErr target.EscalationError
	if errors.As(err, &escErr) {
		return EscalationFailedError{EscalationError: escErr}
	}
	var stagErr target.StagingError
	if errors.As(err, &stagErr) {
		return StagingFailedError{StagingError: stagErr}
	}
	return err
}
