// SPDX-License-Identifier: GPL-3.0-only

package sysctl

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
)

// ReadError is emitted when reading the current sysctl value fails.
type ReadError struct {
	diagnostic.FatalError
	Key    string
	Stderr string
}

func (e ReadError) Error() string {
	return fmt.Sprintf("sysctl read %s failed: %s", e.Key, e.Stderr)
}

func (e ReadError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeReadFailed,
		Text: `sysctl read failed for key "{{.Key}}"`,
		Hint: `check that "{{.Key}}" is a valid sysctl parameter: {{.Stderr}}`,
		Data: e,
	}
}

// WriteError is emitted when setting a sysctl value fails.
type WriteError struct {
	diagnostic.FatalError
	Key    string
	Value  string
	Stderr string
}

func (e WriteError) Error() string {
	return fmt.Sprintf("sysctl write %s=%s failed: %s", e.Key, e.Value, e.Stderr)
}

func (e WriteError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeWriteFailed,
		Text: `sysctl write failed for "{{.Key}}" = "{{.Value}}"`,
		Hint: `stderr: {{.Stderr}}`,
		Data: e,
	}
}

// PersistError is emitted when writing the sysctl.d drop-in file fails.
type PersistError struct {
	diagnostic.FatalError
	Path string
	Err  error
}

func (e PersistError) Error() string {
	return fmt.Sprintf("sysctl persist to %s failed: %s", e.Path, e.Err)
}

func (e PersistError) Unwrap() error { return e.Err }

func (e PersistError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodePersistFailed,
		Text: `failed to write drop-in file "{{.Path}}"`,
		Hint: "ensure /etc/sysctl.d/ exists and is writable",
		Data: e,
	}
}

// CleanupError is emitted when removing a stale sysctl.d drop-in file fails.
type CleanupError struct {
	diagnostic.FatalError
	Path string
	Err  error
}

func (e CleanupError) Error() string {
	return fmt.Sprintf("sysctl cleanup of %s failed: %s", e.Path, e.Err)
}

func (e CleanupError) Unwrap() error { return e.Err }

func (e CleanupError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeCleanupFailed,
		Text: `failed to remove stale drop-in file "{{.Path}}"`,
		Hint: "ensure /etc/sysctl.d/ is writable",
		Data: e,
	}
}
