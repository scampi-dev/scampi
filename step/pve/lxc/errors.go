// SPDX-License-Identifier: GPL-3.0-only

package lxc

import (
	"fmt"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

type InvalidConfigError struct {
	Field  string
	Reason string
	Source spec.SourceSpan
}

func (e InvalidConfigError) Error() string {
	return fmt.Sprintf("invalid pve.lxc config: %s: %s", e.Field, e.Reason)
}

func (e InvalidConfigError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeInvalidConfig,
			Text:   `invalid pve.lxc config: {{.Field}}`,
			Hint:   "{{.Reason}}",
			Data:   e,
			Source: &e.Source,
		},
	}
}

type CommandFailedError struct {
	Op     string
	VMID   int
	Stderr string
	Source spec.SourceSpan
}

func (e CommandFailedError) Error() string {
	return fmt.Sprintf("pve.lxc %s VMID %d failed: %s", e.Op, e.VMID, e.Stderr)
}

func (e CommandFailedError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeCommandFailed,
			Text:   `pve.lxc {{.Op}} VMID {{.VMID}} failed`,
			Help:   "{{.Stderr}}",
			Data:   e,
			Source: &e.Source,
		},
	}
}

type TemplateNotFoundError struct {
	Template string
	Storage  string
	Source   spec.SourceSpan
}

func (e TemplateNotFoundError) Error() string {
	return fmt.Sprintf("template %q not found on storage %q and not available for download", e.Template, e.Storage)
}

func (e TemplateNotFoundError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeTemplateNotFound,
			Text: `template "{{.Template}}" not found`,
			Hint: `not on storage "{{.Storage}}" and not in pveam available` +
				` — check the template name or upload it manually`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

type SizeTruncatedWarning struct {
	Input   string
	Rounded string
	Exact   string
	Source  spec.SourceSpan
}

func (e SizeTruncatedWarning) Error() string {
	return fmt.Sprintf("size %s truncated to %s", e.Input, e.Rounded)
}

func (e SizeTruncatedWarning) Diagnostic() event.Event {
	return event.Warning{
		Template: event.Template{
			ID:     CodeSizeTruncated,
			Text:   `size {{.Input}} truncated to {{.Rounded}} (PVE sizes are whole GiB)`,
			Hint:   `use {{.Rounded}} or {{.Exact}} for precision`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

type SSHKeysSkippedWarning struct {
	VMID   int
	Source spec.SourceSpan
}

func (e SSHKeysSkippedWarning) Error() string {
	return fmt.Sprintf("SSH keys skipped for VMID %d — container is not running", e.VMID)
}

func (e SSHKeysSkippedWarning) Diagnostic() event.Event {
	return event.Warning{
		Template: event.Template{
			ID:     CodeSSHKeysSkipped,
			Text:   `SSH keys skipped for VMID {{.VMID}} — container is not running`,
			Hint:   "SSH keys can only be managed on running containers",
			Data:   e,
			Source: &e.Source,
		},
	}
}

type NodeMismatchError struct {
	Declared string
	Actual   string
	Source   spec.SourceSpan
}

func (e NodeMismatchError) Error() string {
	return fmt.Sprintf("pve.lxc node mismatch: declared %q but connected to %q", e.Declared, e.Actual)
}

func (e NodeMismatchError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeNodeMismatch,
			Text:   `node mismatch: declared "{{.Declared}}" but connected to "{{.Actual}}"`,
			Hint:   `change node to "{{.Actual}}" or connect to "{{.Declared}}"`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

type BindSourceMissingError struct {
	Path   string
	Source spec.SourceSpan
}

func (e BindSourceMissingError) Error() string {
	return fmt.Sprintf("bind mount source %q does not exist on host", e.Path)
}

func (e BindSourceMissingError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeBindSourceMissing,
			Text:   `bind mount source "{{.Path}}" does not exist on host`,
			Hint:   `create "{{.Path}}" on the PVE host first, e.g. with posix.dir`,
			Data:   e,
			Source: &e.Source,
		},
	}
}

func (e BindSourceMissingError) DeferredResource() spec.Resource {
	return spec.PathResource(e.Path)
}

type ImmutableFieldError struct {
	Field   string
	Current string
	Desired string
	Source  spec.SourceSpan
}

func (e ImmutableFieldError) Error() string {
	return fmt.Sprintf(
		"pve.lxc field %q is immutable: %s → %s (destroy and recreate to change)",
		e.Field, e.Current, e.Desired,
	)
}

func (e ImmutableFieldError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeImmutableField,
			Text:   `pve.lxc field "{{.Field}}" cannot be changed ({{.Current}} → {{.Desired}})`,
			Hint:   "destroy and recreate the container to change this field",
			Data:   e,
			Source: &e.Source,
		},
	}
}

type ResizeShrinkError struct {
	Current string
	Desired string
	Source  spec.SourceSpan
}

func (e ResizeShrinkError) Error() string {
	return fmt.Sprintf("pve.lxc rootfs cannot shrink: %s → %s", e.Current, e.Desired)
}

func (e ResizeShrinkError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeResizeShrink,
			Text:   `pve.lxc rootfs cannot shrink ({{.Current}} → {{.Desired}})`,
			Hint:   "PVE only supports growing the rootfs, not shrinking",
			Data:   e,
			Source: &e.Source,
		},
	}
}

type UnsupportedStateError struct {
	State  string
	Source spec.SourceSpan
}

func (e UnsupportedStateError) Error() string {
	return fmt.Sprintf("pve.lxc state %q is not yet supported", e.State)
}

func (e UnsupportedStateError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeUnsupportedState,
			Text:   `pve.lxc state "{{.State}}" is not yet supported`,
			Hint:   "supported states: running, stopped",
			Data:   e,
			Source: &e.Source,
		},
	}
}
