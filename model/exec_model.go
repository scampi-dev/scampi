// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=OpOutcome
package model

import "scampi.dev/scampi/spec"

type OpOutcome uint8

const (
	OpSucceeded OpOutcome = iota
	OpFailed
	OpAborted
	OpSkipped
	OpWouldChange // Check passed, would execute if applied
)

type ExecutionReport struct {
	Actions []ActionReport
	Err     error // terminal error for the run (abort / failure), if any
}
type ActionReport struct {
	Action spec.Action

	Ops []OpReport

	Summary ActionSummary
}
type OpReport struct {
	Op      spec.Op
	Outcome OpOutcome

	Result *spec.Result
	Err    error
}
type ActionSummary struct {
	Total       int
	Succeeded   int
	Failed      int
	Aborted     int
	Skipped     int
	Changed     int
	WouldChange int
}
