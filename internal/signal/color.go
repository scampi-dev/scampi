// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=ColorMode
package signal

type ColorMode uint8

const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)
