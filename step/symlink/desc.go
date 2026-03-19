// SPDX-License-Identifier: GPL-3.0-only

package symlink

import (
	"scampi.dev/scampi/spec"
)

type ensureSymlinkDesc struct {
	Target string
	Link   string
}

func (d ensureSymlinkDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureSymlinkID,
		Text: `symlink "{{.Link}}" -> "{{.Target}}"`,
		Data: d,
	}
}

func (op *ensureSymlinkOp) OpDescription() spec.OpDescription {
	return ensureSymlinkDesc{
		Target: op.target,
		Link:   op.link,
	}
}
