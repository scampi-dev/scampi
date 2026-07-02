// SPDX-License-Identifier: GPL-3.0-only

package cli

import (
	"testing"

	"scampi.dev/scampi/internal/signal"
)

func TestShouldUseColor(t *testing.T) {
	cases := []struct {
		name  string
		mode  signal.ColorMode
		isTTY bool
		want  bool
	}{
		{"auto TTY", signal.ColorAuto, true, true},
		{"auto pipe", signal.ColorAuto, false, false},
		{"always on pipe", signal.ColorAlways, false, true},
		{"never on TTY", signal.ColorNever, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldUseColor(c.mode, c.isTTY); got != c.want {
				t.Errorf("shouldUseColor(%v, tty=%v) = %v, want %v", c.mode, c.isTTY, got, c.want)
			}
		})
	}
}
