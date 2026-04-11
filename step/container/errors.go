// SPDX-License-Identifier: GPL-3.0-only

package container

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

type EmptyImageError struct {
	diagnostic.FatalError
	Source spec.SourceSpan
}

func (e EmptyImageError) Error() string {
	return "container image is required when state is not absent"
}

func (e EmptyImageError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.container.EmptyImage",
		Text:   "container image is required",
		Hint:   `add image = "registry/name:tag"`,
		Data:   e,
		Source: &e.Source,
	}
}

type InvalidMountError struct {
	diagnostic.FatalError
	Got    string
	Reason string
	Source spec.SourceSpan
}

func (e InvalidMountError) Error() string {
	return fmt.Sprintf("invalid mount %q: %s", e.Got, e.Reason)
}

func (e InvalidMountError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.container.InvalidMount",
		Text:   `invalid mount "{{.Got}}"`,
		Hint:   "{{.Reason}}",
		Data:   e,
		Source: &e.Source,
	}
}

type InvalidLabelError struct {
	diagnostic.FatalError
	Key    string
	Reason string
	Source spec.SourceSpan
}

func (e InvalidLabelError) Error() string {
	return fmt.Sprintf("invalid label key %q: %s", e.Key, e.Reason)
}

func (e InvalidLabelError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.container.InvalidLabel",
		Text:   `invalid label key "{{.Key}}"`,
		Hint:   "{{.Reason}}",
		Data:   e,
		Source: &e.Source,
	}
}

type MountSourceMissingError struct {
	diagnostic.FatalError
	Path   string
	Source spec.SourceSpan
}

func (e MountSourceMissingError) Error() string {
	return fmt.Sprintf("mount source directory %q does not exist", e.Path)
}

func (e MountSourceMissingError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.container.MountSourceMissing",
		Text:   `mount source "{{.Path}}" does not exist`,
		Hint:   `add dir(path = "{{.Path}}") before this step`,
		Data:   e,
		Source: &e.Source,
	}
}

func (e MountSourceMissingError) DeferredResource() spec.Resource {
	return spec.PathResource(e.Path)
}

type HealthWaitTimeoutError struct {
	diagnostic.FatalError
	Name   string
	Source spec.SourceSpan
}

func (e HealthWaitTimeoutError) Error() string {
	return fmt.Sprintf("container %q did not become healthy in time", e.Name)
}

func (e HealthWaitTimeoutError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.container.HealthWaitTimeout",
		Text:   `container "{{.Name}}" did not become healthy in time`,
		Hint:   "check container logs for healthcheck failures",
		Data:   e,
		Source: &e.Source,
	}
}

type ContainerUnhealthyError struct {
	diagnostic.FatalError
	Name   string
	Source spec.SourceSpan
}

func (e ContainerUnhealthyError) Error() string {
	return fmt.Sprintf("container %q reported unhealthy", e.Name)
}

func (e ContainerUnhealthyError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.container.Unhealthy",
		Text:   `container "{{.Name}}" reported unhealthy`,
		Hint:   "check container logs for healthcheck failures",
		Data:   e,
		Source: &e.Source,
	}
}

type ContainerCommandError struct {
	diagnostic.FatalError
	Op     string
	Name   string
	Stderr string
	Source spec.SourceSpan
}

func (e ContainerCommandError) Error() string {
	return fmt.Sprintf("container %s %q failed: %s", e.Op, e.Name, e.Stderr)
}

func (e ContainerCommandError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.container.CommandFailed",
		Text:   `container {{.Op}} "{{.Name}}" failed`,
		Help:   "{{.Stderr}}",
		Data:   e,
		Source: &e.Source,
	}
}
