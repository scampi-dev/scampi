// SPDX-License-Identifier: GPL-3.0-only

package pkgmgr

import (
	"scampi.dev/scampi/target"
)

// Detect returns the package manager backend for the given platform, or nil
// if no supported manager is known.
func Detect(platform target.Platform) *Backend {
	if b, ok := backends[platform]; ok {
		return &b
	}
	return nil
}
