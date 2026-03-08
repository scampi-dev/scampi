// SPDX-License-Identifier: GPL-3.0-only

package osutil

import (
	"os"
	"path/filepath"
)

// UserConfigDir resolves the user config directory with XDG_CONFIG_HOME support.
//
// Unlike os.UserConfigDir, this respects $XDG_CONFIG_HOME on every platform
// (including Darwin, where the stdlib ignores it in favor of
// ~/Library/Application Support).
//
// Resolution order:
//  1. $XDG_CONFIG_HOME (if set and absolute)
//  2. os.UserConfigDir() platform default
func UserConfigDir() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" && filepath.IsAbs(dir) {
		return dir, nil
	}
	return os.UserConfigDir()
}
