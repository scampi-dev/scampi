package symlink

import (
	"godoit.dev/doit/spec"
)

type ensureSymlinkDesc struct {
	Target string
	Link   string
}

func (d ensureSymlinkDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   "builtin.symlink",
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
