package template

import (
	"io/fs"

	"godoit.dev/doit/errs"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/step/sharedops/fileops"
)

type (
	Template       struct{}
	TemplateConfig struct {
		Desc    string
		Src     string
		Content string
		Dest    string
		Data    DataConfig
		Perm    string
		Owner   string
		Group   string
	}
	DataConfig struct {
		Values map[string]any
		Env    map[string]string
	}
	templateAction struct {
		idx     int
		desc    string
		kind    string
		src     string
		content string
		dest    string
		data    DataConfig
		mode    fs.FileMode
		owner   string
		group   string
		step    spec.StepInstance
	}
)

func (Template) Kind() string   { return "template" }
func (Template) NewConfig() any { return &TemplateConfig{} }

func (t Template) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*TemplateConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &TemplateConfig{}, step.Config)
	}

	mode, err := fileops.ParsePerm(cfg.Perm, step.Fields["perm"].Value)
	if err != nil {
		return nil, err
	}

	return &templateAction{
		idx:     idx,
		desc:    cfg.Desc,
		kind:    t.Kind(),
		src:     cfg.Src,
		content: cfg.Content,
		dest:    cfg.Dest,
		data:    cfg.Data,
		mode:    mode,
		owner:   cfg.Owner,
		group:   cfg.Group,
		step:    step,
	}, nil
}

func (a *templateAction) Desc() string { return a.desc }
func (a *templateAction) Kind() string { return a.kind }

func (a *templateAction) Ops() []spec.Op {
	render := &renderTemplateOp{
		BaseOp: sharedops.BaseOp{
			SrcSpan:  a.step.Fields["src"].Value,
			DestSpan: a.step.Fields["dest"].Value,
		},
		src:     a.src,
		content: a.content,
		dest:    a.dest,
		data:    a.data,
	}
	chown := &fileops.EnsureOwnerOp{
		BaseOp: sharedops.BaseOp{
			DestSpan: a.step.Fields["dest"].Value,
		},
		Path:  a.dest,
		Owner: a.owner,
		Group: a.group,
	}
	chmod := &fileops.EnsureModeOp{
		BaseOp: sharedops.BaseOp{
			DestSpan: a.step.Fields["dest"].Value,
		},
		Path: a.dest,
		Mode: a.mode,
	}

	render.SetAction(a)
	chown.SetAction(a)
	chmod.SetAction(a)

	chown.AddDependency(render)
	chmod.AddDependency(render)

	return []spec.Op{
		render,
		chown,
		chmod,
	}
}
