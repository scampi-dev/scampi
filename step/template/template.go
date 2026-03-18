// SPDX-License-Identifier: GPL-3.0-only

package template

import (
	"io/fs"
	"path/filepath"
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/step/sharedops/fileops"
)

type (
	Template       struct{}
	TemplateConfig struct {
		_ struct{} `summary:"Render templates with data substitution and owner/permission management"`

		Desc   string         `step:"Human-readable description" optional:"true"`
		Src    spec.SourceRef `step:"Source" example:"local(\"./tmpl\") | inline(\"content\")"`
		Dest   string         `step:"Output file path" example:"/etc/nginx/nginx.conf"`
		Data   DataConfig     `step:"Data sources for template rendering" optional:"true"`
		Perm   string         `step:"File permissions" example:"0644|u=rw,g=r,o=r|rw-r--r--"`
		Owner  string         `step:"Owner user name or UID" example:"root"`
		Group  string         `step:"Group name or GID" example:"root"`
		Verify string         `step:"Validation command (%s = temp file)" optional:"true" example:"nginx -t -c %s"`
	}
	DataConfig struct {
		Values map[string]any
		Env    map[string]string
	}
	templateAction struct {
		idx    int
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
		step   spec.StepInstance
	}
)

func (Template) Kind() string   { return "template" }
func (Template) NewConfig() any { return &TemplateConfig{} }

func (t Template) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*TemplateConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &TemplateConfig{}, step.Config)
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

	if cfg.Verify != "" && !strings.Contains(cfg.Verify, "%s") {
		return nil, sharedops.VerifyMissingPlaceholderError{
			Cmd:    cfg.Verify,
			Source: step.Fields["verify"].Value,
		}
	}

	return &templateAction{
		idx:    idx,
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
		BaseOp: sharedops.BaseOp{
			SrcSpan:  a.step.Fields["src"].Value,
			DestSpan: a.step.Fields["dest"].Value,
		},
		src:    a.src,
		srcRef: a.srcRef,
		dest:   a.dest,
		data:   a.data,
		verify: a.verify,
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
