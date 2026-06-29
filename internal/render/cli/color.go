// SPDX-License-Identifier: GPL-3.0-only

package cli

import "scampi.dev/scampi/internal/render/ansi"

// Semantic colors for CLI output.
var (
	colEngineStarted       = ansi.Green().Dim()
	colEngineFinishedFatal = ansi.BrightRed().Bold()

	colPlanHeader = ansi.Magenta().Bold()
	colPlanRail   = ansi.Magenta().Dim()

	colStepKind              = ansi.Cyan().Bold()
	colStepDesc              = ansi.Cyan()
	colStepRail              = ansi.Cyan()
	colStepOps               = ansi.Cyan().Dim()
	colStepFinishedUnchanged = ansi.Green().Dim()
	colStepFinishedChanged   = ansi.Yellow()

	colOpHeader       = ansi.BrightBlack()
	colOpRail         = ansi.BrightBlack().Dim()
	colOpDesc         = ansi.BrightBlack().Dim()
	colOpCheckUnknown = ansi.Yellow()
	colOpExecChanged  = ansi.BrightBlack()
	colOpExecFailed   = ansi.Red()

	colPlanDeps    = ansi.BrightBlack().Dim()
	colPlanBracket = ansi.BrightBlack().Dim()

	colDiagInfo    = ansi.Blue()
	colDiagWarning = ansi.Yellow()
	colDiagError   = ansi.Red()
	colDiagHelp    = ansi.Cyan()

	colSourceGutter = ansi.BrightBlack()
	colSourceCaret  = ansi.Red()
)
