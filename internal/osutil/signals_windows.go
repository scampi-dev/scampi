// SPDX-License-Identifier: GPL-3.0-only

//go:build windows

package osutil

import "os"

var MainContextSignals = []os.Signal{
	os.Interrupt, // Ctrl+C
}
