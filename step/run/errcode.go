// SPDX-License-Identifier: GPL-3.0-only

package run

import "scampi.dev/scampi/errs"

const (
	CodeApplyFailed          errs.Code = "step.run.ApplyFailed"
	CodePostApplyCheckFailed errs.Code = "step.run.PostApplyCheckFailed"
	CodeCheckAlwaysConflict  errs.Code = "step.run.CheckAlwaysConflict"
	CodeMissingCheckOrAlways errs.Code = "step.run.MissingCheckOrAlways"
)
