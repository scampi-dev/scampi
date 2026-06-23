// SPDX-License-Identifier: GPL-3.0-only

package sysctl

import (
	"fmt"

	"scampi.dev/scampi/internal/diagnostic/event"
)

// ReadError is emitted when reading the current sysctl value fails.
type ReadError struct {
	Key    string
	Stderr string
}

func (e ReadError) Error() string {
	return fmt.Sprintf("sysctl read %s failed: %s", e.Key, e.Stderr)
}

func (e ReadError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeReadFailed,
			Text: `sysctl read failed for key "{{.Key}}"`,
			Hint: `verify "{{.Key}}" is a valid sysctl parameter on this kernel; check the spelling and namespace`,
			Help: `{{.Stderr}}`,
			Data: e,
		},
	}
}

// WriteError is emitted when setting a sysctl value fails.
type WriteError struct {
	Key    string
	Value  string
	Stderr string
}

func (e WriteError) Error() string {
	return fmt.Sprintf("sysctl write %s=%s failed: %s", e.Key, e.Value, e.Stderr)
}

func (e WriteError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeWriteFailed,
			Text: `sysctl write failed for "{{.Key}}" = "{{.Value}}"`,
			Hint: `confirm scampi has privileges to write sysctl values and that "{{.Value}}" is valid for "{{.Key}}"`,
			Help: `{{.Stderr}}`,
			Data: e,
		},
	}
}

// PersistError is emitted when writing the sysctl.d drop-in file fails.
type PersistError struct {
	Path string
	Err  error
}

func (e PersistError) Error() string {
	return fmt.Sprintf("sysctl persist to %s failed: %s", e.Path, e.Err)
}

func (e PersistError) Unwrap() error { return e.Err }

func (e PersistError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodePersistFailed,
			Text: `failed to write drop-in file "{{.Path}}"`,
			Hint: "ensure /etc/sysctl.d/ exists and is writable",
			Data: e,
		},
	}
}

// CleanupError is emitted when removing a stale sysctl.d drop-in file fails.
type CleanupError struct {
	Path string
	Err  error
}

func (e CleanupError) Error() string {
	return fmt.Sprintf("sysctl cleanup of %s failed: %s", e.Path, e.Err)
}

func (e CleanupError) Unwrap() error { return e.Err }

func (e CleanupError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeCleanupFailed,
			Text: `failed to remove stale drop-in file "{{.Path}}"`,
			Hint: "ensure /etc/sysctl.d/ is writable",
			Data: e,
		},
	}
}
