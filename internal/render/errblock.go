// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"fmt"
	"io"
	"strings"
)

// Header + indented per-error lines. errors.Join uses "\n"; the
// block keeps it readable instead of one ugly multi-line key=value.
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
		AnsiDark, ts, AnsiReset,
		AnsiYellow, AnsiReset,
		AnsiBold, AnsiYellow, phase, n, noun, AnsiReset, AnsiUndim)
	return err
}

func writeErrorLine(w io.Writer, msg string, colored bool) error {
	if !colored {
		_, err := fmt.Fprintf(w, "    %s\n", msg)
		return err
	}
	_, err := fmt.Fprintf(w, "%s    %s%s\n", AnsiDim, msg, AnsiUndim)
	return err
}
