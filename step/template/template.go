// SPDX-License-Identifier: GPL-3.0-only

package template

import (
	"io/fs"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/perm"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/step/sharedop/fileop"
)

type (
	Template       struct{}
	TemplateConfig struct {
		_ struct{} `summary:"Render templates with data substitution and owner/permission management"`

		Desc     string         `step:"Human-readable description" optional:"true"`
		Src      spec.SourceRef `step:"Source" example:"local(\"./tmpl\") | inline(\"content\")"`
		Dest     string         `step:"Output file path" example:"/etc/nginx/nginx.conf"`
		Data     DataConfig     `step:"Data sources for template rendering" optional:"true"`
		Perm     string         `step:"File permissions" example:"0644|u=rw,g=r,o=r|rw-r--r--"`
		Owner    string         `step:"Owner user name or UID" example:"root"`
		Group    string         `step:"Group name or GID" example:"root"`
		Verify   string         `step:"Validation command (%s = temp file)" optional:"true" example:"nginx -t -c %s"`
		Backup   bool           `step:"Back up existing file to .bak before overwriting" optional:"true"`
		Promises []string       `step:"Cross-deploy resources this step produces" optional:"true"`
		Inputs   []string       `step:"Cross-deploy resources this step consumes" optional:"true"`
	}
	DataConfig struct {
		Values map[string]any
		Env    map[string]string
	}
	templateAction struct {
		desc   string
		kind   string
		src    string
		srcRef spec.SourceRef
		dest   string
		data   DataConfig
		mode   fs.FileMode
		owner  string
		group  string
		verify string
		backup bool
		step   spec.StepInstance
	}
)

func (Template) Kind() string   { return "template" }
func (Template) NewConfig() any { return &TemplateConfig{} }

func (c *TemplateConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (t Template) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*TemplateConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &TemplateConfig{}, step.Config)
	}

	// dest absoluteness, perm format, and verify-placeholder shape
	// are validated at link time by stub attributes (@std.path,
	// @std.filemode, @std.pattern). Perm parsing here remains
	// because the runtime needs the parsed fs.FileMode value.
	mode, err := perm.ParsePerm(cfg.Perm, step.Fields["perm"].Value)
	if err != nil {
		return nil, err
	}

	return &templateAction{
		desc:   cfg.Desc,
		kind:   t.Kind(),
		src:    cfg.Src.Path,
		srcRef: cfg.Src,
		dest:   cfg.Dest,
		data:   cfg.Data,
		mode:   mode,
		owner:  cfg.Owner,
		group:  cfg.Group,
		verify: cfg.Verify,
		backup: cfg.Backup,
		step:   step,
	}, nil
}

func (a *templateAction) Desc() string { return a.desc }
func (a *templateAction) Kind() string { return a.kind }

func (a *templateAction) Inputs() []spec.Resource {
	var r []spec.Resource
	if a.owner != "" {
		r = append(r, spec.UserResource(a.owner))
	}
	if a.group != "" {
		r = append(r, spec.GroupResource(a.group))
	}
	return r
}
func (a *templateAction) SourcePaths() []string {
	return []string{a.src}
}

func (a *templateAction) Promises() []spec.Resource {
	return []spec.Resource{spec.PathResource(a.dest)}
}

func (a *templateAction) Ops() []spec.Op {
	render := &renderTemplateOp{
		BaseOp: sharedop.BaseOp{
			SrcSpan:  a.step.Fields["src"].Value,
			DestSpan: a.step.Fields["dest"].Value,
		},
		src:    a.src,
		srcRef: a.srcRef,
		dest:   a.dest,
		data:   a.data,
		verify: a.verify,
		backup: a.backup,
	}
	chown := &fileop.EnsureOwnerOp{
		BaseOp: sharedop.BaseOp{
			DestSpan: a.step.Fields["dest"].Value,
		},
		Path:      a.dest,
		Owner:     a.owner,
		Group:     a.group,
		OwnerSpan: a.step.Fields["owner"].Value,
		GroupSpan: a.step.Fields["group"].Value,
	}
	chmod := &fileop.EnsureModeOp{
		BaseOp: sharedop.BaseOp{
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

	ops := []spec.Op{render, chown, chmod}
	ops = append(sharedop.ResolveSourceOps(a.srcRef, render, a, a.step.Fields["src"].Value), ops...)

	return ops
}
