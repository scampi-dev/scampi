// SPDX-License-Identifier: GPL-3.0-only

package fileop

import "scampi.dev/scampi/errs"

const (
	CodeInvalidPermission    errs.Code = "step.InvalidPermission"
	CodeOwnerRead            errs.Code = "step.OwnerRead"
	CodeModeRead             errs.Code = "step.ModeRead"
	CodeVerifyFailed         errs.Code = "step.VerifyFailed"
	CodeVerifyPlaceholderErr errs.Code = "step.VerifyPlaceholderError"
	CodeVerifyIOError        errs.Code = "step.VerifyIOError"
)
