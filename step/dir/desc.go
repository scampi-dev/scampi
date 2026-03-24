// SPDX-License-Identifier: GPL-3.0-only

package dir

import (
	"scampi.dev/scampi/spec"
)

type ensureDirDesc struct {
	Path string
}

func (d ensureDirDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   ensureDirID,
		Text: `ensure directory "{{.Path}}"`,
		Data: d,
	}
}

func (op *ensureDirOp) OpDescription() spec.OpDescription {
	return ensureDirDesc{
		Path: op.path,
	}
}

func (op *ensureDirOp) Inspect() []spec.InspectField {
	return []spec.InspectField{
		{Label: "path", Value: op.path},
	}
}
