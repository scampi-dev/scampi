package copy

import (
	"io/fs"

	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/step/sharedops/fileops"
	"godoit.dev/doit/util"
)

type (
	Copy       struct{}
	CopyConfig struct {
		Desc  string
		Src   string
		Dest  string
		Perm  string
		Owner string
		Group string
	}
	copyAction struct {
		idx   int
		desc  string
		kind  string
		src   string
		dest  string
		mode  fs.FileMode
		owner string
		group string
		step  spec.StepInstance
	}
)

func (Copy) Kind() string   { return "copy" }
func (Copy) NewConfig() any { return &CopyConfig{} }

func (c Copy) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*CopyConfig)
	if !ok {
		return nil, util.BUG("expected %T got %T", &CopyConfig{}, step.Config)
	}

	mode, err := parsePerm(cfg.Perm, step.Fields["perm"].Value)
	if err != nil {
		return nil, err
	}

	return &copyAction{
		idx:   idx,
		desc:  cfg.Desc,
		kind:  c.Kind(),
		src:   cfg.Src,
		dest:  cfg.Dest,
		mode:  mode,
		owner: cfg.Owner,
		group: cfg.Group,

		step: step,
	}, nil
}

func (c *copyAction) Desc() string { return c.desc }
func (c *copyAction) Kind() string { return c.kind }

func (c *copyAction) Ops() []spec.Op {
	cp := &copyFileOp{
		BaseOp: sharedops.BaseOp{
			SrcSpan:  c.step.Fields["src"].Value,
			DestSpan: c.step.Fields["dest"].Value,
		},
		src:  c.src,
		dest: c.dest,
	}
	chown := &fileops.EnsureOwnerOp{
		BaseOp: sharedops.BaseOp{
			DestSpan: c.step.Fields["dest"].Value,
		},
		Path:  c.dest,
		Owner: c.owner,
		Group: c.group,
	}
	chmod := &fileops.EnsureModeOp{
		BaseOp: sharedops.BaseOp{
			DestSpan: c.step.Fields["dest"].Value,
		},
		Path: c.dest,
		Mode: c.mode,
	}

	cp.SetAction(c)
	chown.SetAction(c)
	chmod.SetAction(c)

	chown.AddDependency(cp)
	chmod.AddDependency(cp)

	return []spec.Op{
		cp,
		chown,
		chmod,
	}
}
