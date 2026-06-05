// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"fmt"
	"io"
	"strings"
)

// renderSnapshotRejected writes a multi-line block for a snapshot
// rejection: a header naming the phase and error count, then one
// indented line per joined error. Engine aggregates via errors.Join,
// which separates with "\n"; the block keeps those errors readable
// without breaking the per-line stream contract for the rest of the
// log.
func renderSnapshotRejected(w io.Writer, ts, phase, joined string, colored bool) error {
	errs := splitErrors(joined)
	n := len(errs)
	if err := writeRejectedHeader(w, ts, phase, n, colored); err != nil {
		return err
	}
	for _, e := range errs {
		if err := writeErrorLine(w, e, colored); err != nil {
			return err
		}
	}
	return nil
}

func writeErrorLine(w io.Writer, msg string, colored bool) error {
	if !colored {
		_, err := fmt.Fprintf(w, "    %s\n", msg)
		return err
	}
	_, err := fmt.Fprintf(w, "%s    %s%s\n", ansiDim, msg, ansiUndim)
	return err
}

func splitErrors(joined string) []string {
	trimmed := strings.TrimRight(joined, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func writeRejectedHeader(w io.Writer, ts, phase string, n int, colored bool) error {
	noun := "error"
	if n != 1 {
		noun = "errors"
	}
	if !colored {
		_, err := fmt.Fprintf(w, "%s WRN snapshot rejected at %s (%d %s)\n", ts, phase, n, noun)
		return err
	}
	_, err := fmt.Fprintf(w, "%s%s%s %sWRN%s %s%ssnapshot rejected at %s (%d %s)%s%s\n",
		ansiDark, ts, ansiReset,
		ansiYellow, ansiReset,
		ansiBold, ansiYellow, phase, n, noun, ansiReset, ansiUndim)
	return err
}
