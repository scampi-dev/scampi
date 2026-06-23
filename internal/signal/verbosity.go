// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=Verbosity
package signal

type Verbosity uint8

const (
	Quiet Verbosity = iota // default
	V
	VV
	VVV
)
