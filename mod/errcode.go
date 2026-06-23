// SPDX-License-Identifier: GPL-3.0-only

package mod

import "scampi.dev/scampi/errs"

// Diagnostic codes for mod errors. These are stable identifiers
// surfaced to the error reference docs on scampi.dev — do not rename
// without updating downstream consumers.
const (
	CodeParseError errs.Code = "mod.ParseError"
)
