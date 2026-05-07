// SPDX-License-Identifier: GPL-3.0-only

package runset

import "scampi.dev/scampi/errs"

const (
	CodeListFailed       errs.Code = "step.run_set.ListFailed"
	CodeAddFailed        errs.Code = "step.run_set.AddFailed"
	CodeRemoveFailed     errs.Code = "step.run_set.RemoveFailed"
	CodeInitFailed       errs.Code = "step.run_set.InitFailed"
	CodeMissingTemplate  errs.Code = "step.run_set.MissingTemplate"
	CodeInvalidTemplate  errs.Code = "step.run_set.InvalidTemplate"
	CodeNothingToDeclare errs.Code = "step.run_set.NothingToDeclare"
)
