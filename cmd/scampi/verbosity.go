// SPDX-License-Identifier: GPL-3.0-only

package main

// Verbosity wraps the -v / --quiet level the renderers consume.
// Renderers compare directly with the constants below to gate which
// event categories they admit.
type Verbosity int

const (
	VerbosityQuiet   Verbosity = -1 // --quiet: drop log.info noise
	VerbosityDefault Verbosity = 0  // default operator view
	VerbosityVerbose Verbosity = 1  // -v: also show log.debug
	VerbosityTrace   Verbosity = 2  // -vv: reserved for future trace
)
