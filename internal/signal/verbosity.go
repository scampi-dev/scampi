// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=Verbosity
package signal

// Verbosity is the output ladder: Quiet (one line when converged), V (why:
// all step verdicts), VV (how: op-level detail). The ladder deliberately ends
// at VV - add a level only when there is genuinely more to show.
type Verbosity uint8

const (
	Quiet Verbosity = iota // default
	V
	VV
)
