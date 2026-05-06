// SPDX-License-Identifier: GPL-3.0-only

package copy

import (
	"io/fs"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/step/sharedop/fileop"
)

var _ spec.StepType = Copy{}

type (
	Copy       struct{}
	CopyConfig struct {
		_ struct{} `summary:"Copy files with owner and permission management"`

		Desc     string         `step:"Human-readable description" optional:"true"`
		Src      spec.SourceRef `step:"Source" example:"local(\"./config.yaml\") | inline(\"content\")"`
		Dest     string         `step:"Destination file path" example:"/etc/app/config.yaml"`
		Perm     string         `step:"File permissions" example:"0644|u=rw,g=r,o=r|rw-r--r--"`
		Owner    string         `step:"Owner user name or UID" example:"root"`
		Group    string         `step:"Group name or GID" example:"root"`
		Verify   string         `step:"Validation command (%s = temp file)" optional:"true" example:"visudo -cf %s"`
		Backup   bool           `step:"Back up existing file to .bak before overwriting" optional:"true"`
		Promises []string       `step:"Cross-deploy resources this step produces" optional:"true"`
		Inputs   []string       `step:"Cross-deploy resources this step consumes" optional:"true"`
	}
	copyAction struct {
		desc   string
		kind   string
		src    string
		srcRef spec.SourceRef
		dest   string
		mode   fs.FileMode
		owner  string
		group  string
		verify string
		backup bool
		step   spec.StepInstance
	}
)

func (Copy) Kind() string   { return "copy" }
func (Copy) NewConfig() any { return &CopyConfig{} }

func (c *CopyConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (c Copy) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*CopyConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &CopyConfig{}, step.Config)
	}

	// dest absoluteness, perm format, and verify-placeholder shape
	// are validated at link time by the @std.path(absolute=true),
	// @std.filemode, and @std.pattern attributes on copy's stub.
	// We still parse perm here because the runtime needs the
	// fs.FileMode value, but a parse error reaching this point
	// indicates a non-literal expression (e.g. perm = std.env(...))
	// that bypassed the static check — fail with the same error
	// shape as the link-time check would have produced.
	mode, err := fileop.ParsePerm(cfg.Perm, step.Fields["perm"].Value)
	if err != nil {
		return nil, err
	}

	return &copyAction{
		desc:   cfg.Desc,
		kind:   c.Kind(),
		src:    cfg.Src.Path,
		srcRef: cfg.Src,
		dest:   cfg.Dest,
		mode:   mode,
		owner:  cfg.Owner,
		group:  cfg.Group,
		verify: cfg.Verify,
		backup: cfg.Backup,

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
		BaseOp: sharedop.BaseOp{
			SrcSpan:  c.step.Fields["src"].Value,
			DestSpan: c.step.Fields["dest"].Value,
		},
		src:    c.src,
		srcRef: c.srcRef,
		dest:   c.dest,
		verify: c.verify,
		backup: c.backup,
	}
	chown := &fileop.EnsureOwnerOp{
		BaseOp: sharedop.BaseOp{
			DestSpan: c.step.Fields["dest"].Value,
		},
		Path:      c.dest,
		Owner:     c.owner,
		Group:     c.group,
		OwnerSpan: c.step.Fields["owner"].Value,
		GroupSpan: c.step.Fields["group"].Value,
	}
	chmod := &fileop.EnsureModeOp{
		BaseOp: sharedop.BaseOp{
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
	ops = append(sharedop.ResolveSourceOps(c.srcRef, cp, c, c.step.Fields["src"].Value), ops...)

	return ops
}
