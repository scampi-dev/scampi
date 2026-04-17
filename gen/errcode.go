// SPDX-License-Identifier: GPL-3.0-only

package gen

import "scampi.dev/scampi/errs"

// Diagnostic codes for code generation events.
const (
	CodeWarning errs.Code = "gen.Warning"
	CodeError   errs.Code = "gen.Error"
)
