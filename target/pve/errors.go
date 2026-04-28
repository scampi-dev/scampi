// SPDX-License-Identifier: GPL-3.0-only

package pve

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
)

// PctFailedError is returned when a pct subcommand exits non-zero.
type PctFailedError struct {
	diagnostic.FatalError
	Op       string // e.g. "exec", "push", "pull"
	VMID     int
	ExitCode int
	Stderr   string
}

func (e PctFailedError) Error() string {
	return fmt.Sprintf("pct %s for VMID %d failed: %s", e.Op, e.VMID, e.Stderr)
}

func (e PctFailedError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodePctFailed,
		Text: `pct {{.Op}} for VMID {{.VMID}} failed`,
		Help: "{{.Stderr}}",
		Data: e,
	}
}
