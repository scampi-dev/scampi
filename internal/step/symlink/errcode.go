// SPDX-License-Identifier: GPL-3.0-only

package symlink

import "scampi.dev/scampi/internal/errs"

const (
	CodeLinkDirMissing errs.Code = "step.symlink.LinkDirMissing"
	CodeLinkRead       errs.Code = "step.symlink.LinkRead"
)
