// SPDX-License-Identifier: GPL-3.0-only

package cli

import "scampi.dev/scampi/render/ansi"

// Semantic colors for CLI output.
var (
	colEngineStarted           = ansi.Green().Dim()
	colEngineFinishedUnchanged = ansi.Green()
	colEngineFinishedChanged   = ansi.Yellow()
	colEngineFinishedFailed    = ansi.Red()
	colEngineFinishedFatal     = ansi.BrightRed().Bold()

	colPlanHeader             = ansi.Magenta().Bold()
	colPlanRail               = ansi.Magenta().Dim()
	colPlanStarted            = ansi.Blue()
	colPlanStartedUnit        = ansi.Blue().Bold()
	colPlanFinishedOK         = ansi.Blue().Dim()
	colPlanFinishedOKUnit     = ansi.Blue().Dim().Bold()
	colPlanFinishedFailed     = ansi.Red()
	colPlanFinishedFailedUnit = ansi.Red().Bold()
	colPlanStepPlanned        = ansi.BrightBlack().Dim()

	colActionKind              = ansi.Cyan().Bold()
	colActionDesc              = ansi.Cyan()
	colActionRail              = ansi.Cyan()
	colActionOps               = ansi.Cyan().Dim()
	colActionFinishedUnchanged = ansi.Green().Dim()
	colActionFinishedChanged   = ansi.Yellow()
	colActionFinishedFailed    = ansi.Red()

	colOpHeader           = ansi.BrightBlack()
	colOpRail             = ansi.BrightBlack().Dim()
	colOpDesc             = ansi.BrightBlack().Dim()
	colOpCheckSatisfied   = ansi.BrightBlack().Dim()
	colOpCheckUnsatisfied = ansi.BrightBlack().Dim()
	colOpDrift            = ansi.BrightBlack().Dim()
	colOpCheckUnknown     = ansi.Yellow()
	colOpExecChanged      = ansi.BrightBlack()
	colOpExecFailed       = ansi.Red()

	colPlanDeps    = ansi.BrightBlack().Dim()
	colPlanBracket = ansi.BrightBlack().Dim()

	colSpinner = ansi.Cyan().Dim()

	colDiagMsg      = ansi.Red()
	colDiagHelp     = ansi.Cyan()
	colSourceGutter = ansi.BrightBlack()
	colSourceCaret  = ansi.Red()
)
