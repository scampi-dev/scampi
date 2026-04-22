// SPDX-License-Identifier: GPL-3.0-only

package lxc

import "scampi.dev/scampi/errs"

const (
	CodeInvalidConfig    errs.Code = "step.pve.lxc.InvalidConfig"
	CodeCommandFailed    errs.Code = "step.pve.lxc.CommandFailed"
	CodeUnsupportedState errs.Code = "step.pve.lxc.UnsupportedState"
	CodeTemplateNotFound errs.Code = "step.pve.lxc.TemplateNotFound"
	CodeSizeTruncated    errs.Code = "step.pve.lxc.SizeTruncated"
	CodeImmutableField   errs.Code = "step.pve.lxc.ImmutableField"
	CodeResizeShrink     errs.Code = "step.pve.lxc.ResizeShrink"
)
