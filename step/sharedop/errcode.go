// SPDX-License-Identifier: GPL-3.0-only

package sharedop

import "scampi.dev/scampi/errs"

const (
	CodeUnknownUser      errs.Code = "step.UnknownUser"
	CodeUnknownGroup     errs.Code = "step.UnknownGroup"
	CodePermissionDenied errs.Code = "step.PermissionDenied"

	CodeDownloadError    errs.Code = "step.download.Error"
	CodeChecksumMismatch errs.Code = "step.download.ChecksumMismatch"

	CodeEscalationFailed  errs.Code = "target.EscalationFailed"
	CodeEscalationMissing errs.Code = "target.EscalationMissing"
	CodeStagingFailed     errs.Code = "target.StagingFailed"
)
