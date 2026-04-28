// SPDX-License-Identifier: GPL-3.0-only

package engine

import "scampi.dev/scampi/errs"

// Diagnostic codes for engine errors. These are stable identifiers
// surfaced to the LSP and (eventually) the error reference docs on
// scampi.dev — do not rename without updating downstream consumers.
const (
	CodeLoadConfigError     errs.Code = "engine.LoadConfigError"
	CodeCapabilityMismatch  errs.Code = "engine.CapabilityMismatch"
	CodeUnknownHook         errs.Code = "engine.UnknownHook"
	CodeHookCycle           errs.Code = "engine.HookCycle"
	CodeCyclicDependency    errs.Code = "engine.CyclicDependency"
	CodeActionCyclicDep     errs.Code = "engine.ActionCyclicDependency"
	CodeRefError            errs.Code = "engine.RefError"
	CodeNoDiffableOps       errs.Code = "engine.inspect.NoDiffableOps"
	CodeMultipleDiffableOps errs.Code = "engine.inspect.MultipleDiffableOps"
	CodeDuplicateResource   errs.Code = "engine.DuplicateResource"
	CodeUnknownIndexKind    errs.Code = "index.UnknownKind"
	CodeUnknownDeployBlock  errs.Code = "config.UnknownDeployBlock"
	CodeNoDeployBlocks      errs.Code = "config.NoDeployBlocks"
	CodeNoTargetsInDeploy   errs.Code = "config.NoTargetsInDeploy"
	CodeUnknownTarget       errs.Code = "config.UnknownTarget"
	CodeTargetNotInDeploy   errs.Code = "config.TargetNotInDeploy"
)
