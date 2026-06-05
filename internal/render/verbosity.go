// SPDX-License-Identifier: GPL-3.0-only

package render

type Verbosity int

const (
	VerbosityQuiet   Verbosity = -1
	VerbosityDefault Verbosity = 0
	VerbosityVerbose Verbosity = 1
	VerbosityTrace   Verbosity = 2
)
