// SPDX-License-Identifier: GPL-3.0-only

package testkit

import "scampi.dev/scampi/errs"

// Diagnostic codes for test framework events.
const (
	CodeTestPass    errs.Code = "test.Pass"
	CodeTestFail    errs.Code = "test.Fail"
	CodeTestSummary errs.Code = "test.Summary"
	CodeTestError   errs.Code = "test.Error"
)
