// SPDX-License-Identifier: GPL-3.0-only

package sysctl

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
)

// ReadError is emitted when reading the current sysctl value fails.
type ReadError struct {
	Key    string
	Stderr string
}

func (e ReadError) Error() string {
	return fmt.Sprintf("sysctl read %s failed: %s", e.Key, e.Stderr)
}

func (e ReadError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.sysctl.ReadFailed",
		Text: `sysctl read failed for key "{{.Key}}"`,
		Hint: `check that "{{.Key}}" is a valid sysctl parameter: {{.Stderr}}`,
		Data: e,
	}
}

func (ReadError) Severity() signal.Severity { return signal.Error }
func (ReadError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// WriteError is emitted when setting a sysctl value fails.
type WriteError struct {
	Key    string
	Value  string
	Stderr string
}

func (e WriteError) Error() string {
	return fmt.Sprintf("sysctl write %s=%s failed: %s", e.Key, e.Value, e.Stderr)
}

func (e WriteError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.sysctl.WriteFailed",
		Text: `sysctl write failed for "{{.Key}}" = "{{.Value}}"`,
		Hint: `stderr: {{.Stderr}}`,
		Data: e,
	}
}

func (WriteError) Severity() signal.Severity { return signal.Error }
func (WriteError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// PersistError is emitted when writing the sysctl.d drop-in file fails.
type PersistError struct {
	Path string
	Err  error
}

func (e PersistError) Error() string {
	return fmt.Sprintf("sysctl persist to %s failed: %s", e.Path, e.Err)
}

func (e PersistError) Unwrap() error { return e.Err }

func (e PersistError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.sysctl.PersistFailed",
		Text: `failed to write drop-in file "{{.Path}}"`,
		Hint: "ensure /etc/sysctl.d/ exists and is writable",
		Data: e,
	}
}

func (PersistError) Severity() signal.Severity { return signal.Error }
func (PersistError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// CleanupError is emitted when removing a stale sysctl.d drop-in file fails.
type CleanupError struct {
	Path string
	Err  error
}

func (e CleanupError) Error() string {
	return fmt.Sprintf("sysctl cleanup of %s failed: %s", e.Path, e.Err)
}

func (e CleanupError) Unwrap() error { return e.Err }

func (e CleanupError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.sysctl.CleanupFailed",
		Text: `failed to remove stale drop-in file "{{.Path}}"`,
		Hint: "ensure /etc/sysctl.d/ is writable",
		Data: e,
	}
}

func (CleanupError) Severity() signal.Severity { return signal.Error }
func (CleanupError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
