// SPDX-License-Identifier: GPL-3.0-only

package mod

import "scampi.dev/scampi/errs"

// Diagnostic codes for mod errors. These are stable identifiers
// surfaced to the LSP and (eventually) the error reference docs on
// scampi.dev — do not rename without updating downstream consumers.
const (
	CodeParseError      errs.Code = "mod.ParseError"
	CodeNotFound        errs.Code = "mod.NotFound"
	CodeNotCached       errs.Code = "mod.NotCached"
	CodeNoEntryPoint    errs.Code = "mod.NoEntryPoint"
	CodeInfo            errs.Code = "mod.Info"
	CodeWriteError      errs.Code = "mod.WriteError"
	CodeInitError       errs.Code = "mod.InitError"
	CodeTidyError       errs.Code = "mod.TidyError"
	CodeSumError        errs.Code = "mod.SumError"
	CodeFetchError      errs.Code = "mod.FetchError"
	CodeNotAModule      errs.Code = "mod.NotAModule"
	CodeAddError        errs.Code = "mod.AddError"
	CodeNoStableVersion errs.Code = "mod.NoStableVersion"
	CodeCycleError      errs.Code = "mod.CycleError"
	CodeSumMismatch     errs.Code = "mod.SumMismatch"
)
