// SPDX-License-Identifier: GPL-3.0-only

package template

import (
	"io/fs"
	"path/filepath"

	"godoit.dev/doit/errs"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/sharedops"
	"godoit.dev/doit/step/sharedops/fileops"
)

type (
	Template       struct{}
	TemplateConfig struct {
		_ struct{} `summary:"Render templates with data substitution and owner/permission management"`

		Desc    string     `step:"Human-readable description" optional:"true"`
		Src     string     `step:"Source template file (exclusive with content)" optional:"true" example:"./tmpl"`
		Content string     `step:"Inline template string (exclusive with src)" optional:"true"`
		Dest    string     `step:"Output file path" example:"/etc/nginx/nginx.conf"`
		Data    DataConfig `step:"Data sources for template rendering" optional:"true"`
		Perm    string     `step:"File permissions" example:"0644|u=rw,g=r,o=r|rw-r--r--"`
		Owner   string     `step:"Owner user name or UID" example:"root"`
		Group   string     `step:"Group name or GID" example:"root"`
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

func (c *TemplateConfig) Validate(step spec.StepInstance) error {
	hasSrc := c.Src != ""
	hasContent := c.Content != ""
	if hasSrc == hasContent {
		var got []string
		source := step.Source
		if hasSrc {
			got = []string{"src", "content"}
			source = step.Fields["content"].Value
		}
		return MutuallyExclusiveError{
			Fields: []string{"src", "content"},
			Got:    got,
			Source: source,
		}
	}
	return nil
}

func (t Template) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*TemplateConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &TemplateConfig{}, step.Config)
	}

	if err := cfg.Validate(step); err != nil {
		return nil, err
	}

	if !filepath.IsAbs(cfg.Dest) {
		return nil, sharedops.RelativePathError{
			Field:  "dest",
			Path:   cfg.Dest,
			Source: step.Fields["dest"].Value,
		}
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

func (a *templateAction) InputPaths() []string {
	if a.src != "" {
		return []string{a.src}
	}
	return nil
}

func (a *templateAction) OutputPaths() []string { return []string{a.dest} }

func (a *templateAction) Ops() []spec.Op {
	render := &renderTemplateOp{
		BaseOp: sharedops.BaseOp{
			SrcSpan:  a.step.Fields["src"].Value,
			DestSpan: a.step.Fields["dest"].Value,
		},
		src:         a.src,
		content:     a.content,
		contentSpan: a.step.Fields["content"].Value,
		dest:        a.dest,
		data:        a.data,
	}
	chown := &fileops.EnsureOwnerOp{
		BaseOp: sharedops.BaseOp{
			DestSpan: a.step.Fields["dest"].Value,
		},
		Path:      a.dest,
		Owner:     a.owner,
		Group:     a.group,
		OwnerSpan: a.step.Fields["owner"].Value,
		GroupSpan: a.step.Fields["group"].Value,
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
