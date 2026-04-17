// SPDX-License-Identifier: GPL-3.0-only

package mount

import "scampi.dev/scampi/errs"

const (
	CodeCommandFailed errs.Code = "step.mount.CommandFailed"
	CodeMissingTool   errs.Code = "step.mount.MissingTool"
)
