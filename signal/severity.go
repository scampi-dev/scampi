// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=Severity
package signal

// Severity is the three-level severity carried by diagnostics.
// Richer notions (e.g. abort vs. non-abort) live on orthogonal axes
// (e.g. diagnostic.Impact) - this enum stays small on purpose.
type Severity uint8

const (
	Info Severity = iota
	Warning
	Error
)
