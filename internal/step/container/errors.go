// SPDX-License-Identifier: GPL-3.0-only

package container

import (
	"fmt"

	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/spec"
)

type InvalidHealthcheckError struct {
	Field string
	Value string
	Err   error
}

func (e InvalidHealthcheckError) Error() string {
	return fmt.Sprintf("invalid healthcheck %s %q: %v", e.Field, e.Value, e.Err)
}

func (e InvalidHealthcheckError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeInvalidHealthcheck,
			Text: `invalid healthcheck {{.Field}} "{{.Value}}"`,
			Hint: `use a Go duration string, e.g. {{.Field}} = "30s"`,
			Data: e,
		},
	}
}

type EmptyImageError struct {
	Span spec.Span
}

func (e EmptyImageError) Error() string {
	return "container image is required when state is not absent"
}

func (e EmptyImageError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeEmptyImage,
			Text: "container image is required",
			Hint: `add image = "registry/name:tag"`,
			Data: e,
			Span: &e.Span,
		},
	}
}

type InvalidMountError struct {
	Got    string
	Reason string
	Span   spec.Span
}

func (e InvalidMountError) Error() string {
	return fmt.Sprintf("invalid mount %q: %s", e.Got, e.Reason)
}

func (e InvalidMountError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeInvalidMount,
			Text: `invalid mount "{{.Got}}"`,
			Hint: "{{.Reason}}",
			Data: e,
			Span: &e.Span,
		},
	}
}

type InvalidLabelError struct {
	Key    string
	Reason string
	Span   spec.Span
}

func (e InvalidLabelError) Error() string {
	return fmt.Sprintf("invalid label key %q: %s", e.Key, e.Reason)
}

func (e InvalidLabelError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeInvalidLabel,
			Text: `invalid label key "{{.Key}}"`,
			Hint: "{{.Reason}}",
			Data: e,
			Span: &e.Span,
		},
	}
}

type MountSourceMissingError struct {
	Path string
	Span spec.Span
}

func (e MountSourceMissingError) Error() string {
	return fmt.Sprintf("mount source directory %q does not exist", e.Path)
}

func (e MountSourceMissingError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeMountSourceMissing,
			Text: `mount source "{{.Path}}" does not exist`,
			Hint: `add dir(path = "{{.Path}}") before this step`,
			Data: e,
			Span: &e.Span,
		},
	}
}

func (e MountSourceMissingError) DeferredResource() spec.Resource {
	return spec.PathResource(e.Path)
}

type HealthWaitTimeoutError struct {
	Name string
	Span spec.Span
}

func (e HealthWaitTimeoutError) Error() string {
	return fmt.Sprintf("container %q did not become healthy in time", e.Name)
}

func (e HealthWaitTimeoutError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeHealthWaitTimeout,
			Text: `container "{{.Name}}" did not become healthy in time`,
			Hint: "check container logs for healthcheck failures",
			Data: e,
			Span: &e.Span,
		},
	}
}

type ContainerUnhealthyError struct {
	Name string
	Span spec.Span
}

func (e ContainerUnhealthyError) Error() string {
	return fmt.Sprintf("container %q reported unhealthy", e.Name)
}

func (e ContainerUnhealthyError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeUnhealthy,
			Text: `container "{{.Name}}" reported unhealthy`,
			Hint: "check container logs for healthcheck failures",
			Data: e,
			Span: &e.Span,
		},
	}
}

type ContainerCommandError struct {
	Op     string
	Name   string
	Stderr string
	Span   spec.Span
}

func (e ContainerCommandError) Error() string {
	return fmt.Sprintf("container %s %q failed: %s", e.Op, e.Name, e.Stderr)
}

func (e ContainerCommandError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeCommandFailed,
			Text: `container {{.Op}} "{{.Name}}" failed`,
			Help: "{{.Stderr}}",
			Data: e,
			Span: &e.Span,
		},
	}
}
