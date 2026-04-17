// SPDX-License-Identifier: GPL-3.0-only

package sysctl

import "scampi.dev/scampi/errs"

const (
	CodeReadFailed    errs.Code = "step.sysctl.ReadFailed"
	CodeWriteFailed   errs.Code = "step.sysctl.WriteFailed"
	CodePersistFailed errs.Code = "step.sysctl.PersistFailed"
	CodeCleanupFailed errs.Code = "step.sysctl.CleanupFailed"
)
