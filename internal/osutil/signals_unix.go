// SPDX-License-Identifier: GPL-3.0-only

//go:build unix

package osutil

import (
	"os"
	"syscall"
)

var MainContextSignals = []os.Signal{
	syscall.SIGINT,  // (Ctrl+C)
	syscall.SIGTERM, // graceful termination
}
