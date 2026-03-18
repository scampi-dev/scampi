// SPDX-License-Identifier: GPL-3.0-only

package copy

import (
	"io/fs"
	"path/filepath"
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/step/sharedops/fileops"
)

var _ spec.StepType = Copy{}

type (
	Copy       struct{}
	CopyConfig struct {
		_ struct{} `summary:"Copy files with owner and permission management"`

		Desc   string         `step:"Human-readable description" optional:"true"`
		Src    spec.SourceRef `step:"Source" example:"local(\"./config.yaml\") | inline(\"content\")"`
		Dest   string         `step:"Destination file path" example:"/etc/app/config.yaml"`
		Perm   string         `step:"File permissions" example:"0644|u=rw,g=r,o=r|rw-r--r--"`
		Owner  string         `step:"Owner user name or UID" example:"root"`
		Group  string         `step:"Group name or GID" example:"root"`
		Verify string         `step:"Validation command (%s = temp file)" optional:"true" example:"visudo -cf %s"`
	}
	copyAction struct {
		idx    int
		desc   string
		kind   string
		src    string
		srcRef spec.SourceRef
		dest   string
		mode   fs.FileMode
		owner  string
		group  string
		verify string
		step   spec.StepInstance
	}
)

func (Copy) Kind() string   { return "copy" }
func (Copy) NewConfig() any { return &CopyConfig{} }

func (c Copy) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*CopyConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &CopyConfig{}, step.Config)
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

	return &copyAction{
		idx:    idx,
		desc:   cfg.Desc,
		kind:   c.Kind(),
		src:    cfg.Src.Path,
		srcRef: cfg.Src,
		dest:   cfg.Dest,
		mode:   mode,
		owner:  cfg.Owner,
		group:  cfg.Group,
		verify: cfg.Verify,

		step: step,
	}, nil
}

func (c *copyAction) Desc() string { return c.desc }
func (c *copyAction) Kind() string { return c.kind }

func (c *copyAction) Inputs() []spec.Resource {
	var r []spec.Resource
	if c.owner != "" {
		r = append(r, spec.UserResource(c.owner))
	}
	if c.group != "" {
		r = append(r, spec.GroupResource(c.group))
	}
	return r
}
func (c *copyAction) SourcePaths() []string {
	return []string{c.src}
}
func (c *copyAction) Promises() []spec.Resource { return []spec.Resource{spec.PathResource(c.dest)} }

func (c *copyAction) Ops() []spec.Op {
	cp := &copyFileOp{
		BaseOp: sharedops.BaseOp{
			SrcSpan:  c.step.Fields["src"].Value,
			DestSpan: c.step.Fields["dest"].Value,
		},
		src:    c.src,
		srcRef: c.srcRef,
		dest:   c.dest,
		verify: c.verify,
	}
	chown := &fileops.EnsureOwnerOp{
		BaseOp: sharedops.BaseOp{
			DestSpan: c.step.Fields["dest"].Value,
		},
		Path:      c.dest,
		Owner:     c.owner,
		Group:     c.group,
		OwnerSpan: c.step.Fields["owner"].Value,
		GroupSpan: c.step.Fields["group"].Value,
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

	ops := []spec.Op{cp, chown, chmod}
	ops = append(sharedops.ResolveSourceOps(c.srcRef, cp, c, c.step.Fields["src"].Value), ops...)

	return ops
}
