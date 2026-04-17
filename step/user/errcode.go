// SPDX-License-Identifier: GPL-3.0-only

package user

import "scampi.dev/scampi/errs"

const (
	CodeCreateFailed errs.Code = "step.user.CreateFailed"
	CodeModifyFailed errs.Code = "step.user.ModifyFailed"
	CodeDeleteFailed errs.Code = "step.user.DeleteFailed"
)
