// SPDX-License-Identifier: GPL-3.0-only

package service

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// ServiceCommandError is emitted when a service command (start/stop/enable/disable) fails.
type ServiceCommandError struct {
	diagnostic.FatalError
	Op     string
	Name   string
	Stderr string
	Source spec.SourceSpan
}

func (e ServiceCommandError) Error() string {
	return fmt.Sprintf("failed to %s service %s", e.Op, e.Name)
}

func (e ServiceCommandError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.service.CommandFailed",
		Text:   `failed to {{.Op}} service {{.Name}}: {{.Stderr}}`,
		Hint:   "check that the service name is correct and the init system is available",
		Help:   "the service command exited with a non-zero status",
		Data:   e,
		Source: &e.Source,
	}
}

// DaemonReloadError is emitted when daemon-reload fails.
type DaemonReloadError struct {
	diagnostic.FatalError
	Name   string
	Stderr string
	Source spec.SourceSpan
}

func (e DaemonReloadError) Error() string {
	return fmt.Sprintf("daemon-reload failed before starting service %s", e.Name)
}

func (e DaemonReloadError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.service.DaemonReloadFailed",
		Text:   `daemon-reload failed before starting service {{.Name}}: {{.Stderr}}`,
		Hint:   "check systemd configuration and permissions",
		Data:   e,
		Source: &e.Source,
	}
}
