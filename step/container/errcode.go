// SPDX-License-Identifier: GPL-3.0-only

package container

import "scampi.dev/scampi/errs"

const (
	CodeEmptyImage         errs.Code = "step.container.EmptyImage"
	CodeInvalidMount       errs.Code = "step.container.InvalidMount"
	CodeInvalidLabel       errs.Code = "step.container.InvalidLabel"
	CodeMountSourceMissing errs.Code = "step.container.MountSourceMissing"
	CodeHealthWaitTimeout  errs.Code = "step.container.HealthWaitTimeout"
	CodeUnhealthy          errs.Code = "step.container.Unhealthy"
	CodeCommandFailed      errs.Code = "step.container.CommandFailed"
)
