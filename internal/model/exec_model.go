// SPDX-License-Identifier: GPL-3.0-only

// Package model re-exports the execution report types from
// internal/diagnostic/result. Temporary shim for the #430 migration; callers
// move to result/ directly in the final sub-phase, after which this package
// is deleted.
package model

import "scampi.dev/scampi/internal/diagnostic/result"

type (
	OpOutcome       = result.OpOutcome
	ExecutionReport = result.Execution
	ActionReport    = result.ActionReport
	OpReport        = result.OpReport
	ActionSummary   = result.ActionSummary
)

const (
	OpSucceeded   = result.OpSucceeded
	OpFailed      = result.OpFailed
	OpAborted     = result.OpAborted
	OpSkipped     = result.OpSkipped
	OpWouldChange = result.OpWouldChange
)
