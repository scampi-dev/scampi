// SPDX-License-Identifier: GPL-3.0-only

package pve

import (
	"fmt"

	"scampi.dev/scampi/diagnostic/event"
)

// PctFailedError is returned when a pct subcommand exits non-zero.
type PctFailedError struct {
	Op       string // e.g. "exec", "push", "pull"
	VMID     int
	ExitCode int
	Stderr   string
}

func (e PctFailedError) Error() string {
	return fmt.Sprintf("pct %s for VMID %d failed: %s", e.Op, e.VMID, e.Stderr)
}

func (e PctFailedError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodePctFailed,
			Text: `pct {{.Op}} for VMID {{.VMID}} failed`,
			Help: "{{.Stderr}}",
			Data: e,
		},
	}
}

// LxcUnreachableError is returned when lazy backend probes can't reach
// the LXC — typically because a sibling deploy block hasn't created or
// started it yet. Surfaced as a fatal action error, not a plan-time
// abort, so the user sees a clean message instead of a cap mismatch.
type LxcUnreachableError struct {
	VMID int
}

func (e LxcUnreachableError) Error() string {
	return fmt.Sprintf("pve.lxc_target: cannot reach LXC %d", e.VMID)
}

func (e LxcUnreachableError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeLxcUnreachable,
			Text: `pve.lxc_target: cannot reach LXC {{.VMID}}`,
			Hint: "ensure the LXC exists and is running before configure-side deploys",
			Help: "this usually means a sibling deploy block that creates or " +
				"starts the LXC hasn't finished yet — deploy blocks run " +
				"concurrently and don't (yet) order around shared resources.",
			Data: e,
		},
	}
}

// BackendMissingError is returned when the LXC is reachable but has no
// suitable backend for the requested operation (e.g. posix.pkg against
// a busybox container that has neither apt nor apk).
type BackendMissingError struct {
	VMID int
	Kind string // "package manager", "service manager", "container runtime"
}

func (e BackendMissingError) Error() string {
	return fmt.Sprintf("pve.lxc_target: no %s inside LXC %d", e.Kind, e.VMID)
}

func (e BackendMissingError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:   CodeBackendMissing,
			Text: `pve.lxc_target: no {{.Kind}} inside LXC {{.VMID}}`,
			Hint: "use a different LXC template, or remove the incompatible step",
			Data: e,
		},
	}
}
