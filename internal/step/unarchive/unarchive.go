// SPDX-License-Identifier: GPL-3.0-only

package unarchive

import (
	"io/fs"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/perm"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/sharedop"
	"scampi.dev/scampi/internal/step/sharedop/fileop"
)

type (
	Unarchive       struct{}
	UnarchiveConfig struct {
		_ struct{} `summary:"Extract an archive to a target directory with optional recursive unpacking"`

		Desc     string         `step:"Human-readable description" optional:"true"`
		Src      spec.SourceRef `step:"Source archive" example:"local(\"./files/site.tar.gz\")"`
		Dest     string         `step:"Target directory for extraction" example:"/var/www/mysite"`
		Depth    int            `step:"Nested archive recursion (-1=unlimited, 0=top only)" optional:"true" default:"0"`
		Owner    string         `step:"Owner applied recursively after extraction" optional:"true" example:"www-data"`
		Group    string         `step:"Group applied recursively after extraction" optional:"true" example:"www-data"`
		Perm     string         `step:"Permissions applied recursively after extraction" optional:"true" example:"0755"`
		Promises []string       `step:"Cross-deploy resources this step produces" optional:"true"`
		Inputs   []string       `step:"Cross-deploy resources this step consumes" optional:"true"`
	}
	unarchiveStep struct {
		desc   string
		kind   string
		src    string
		srcRef spec.SourceRef
		dest   string
		depth  int
		owner  string
		group  string
		mode   fs.FileMode
		format archiveFormat
		step   spec.DeclaredStep
	}
)

func (Unarchive) Kind() string   { return "unarchive" }
func (Unarchive) NewConfig() any { return &UnarchiveConfig{} }

func (c *UnarchiveConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (u Unarchive) Plan(step spec.DeclaredStep) (spec.Step, error) {
	cfg, ok := step.Config.(*UnarchiveConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &UnarchiveConfig{}, step.Config)
	}

	// dest absoluteness and perm format are link-time checks via
	// @std.path(absolute=true) and @std.filemode on the stub.
	// Archive format detection (filename-based) and owner/group
	// mutual requirement are runtime/cross-field — they stay.
	formatPath := cfg.Src.Path
	if cfg.Src.Kind == spec.SourceRemote {
		formatPath = cfg.Src.URL
	}
	fmt, ok := detectFormat(formatPath)
	if !ok {
		return nil, UnsupportedArchiveError{
			Path:   formatPath,
			Source: step.Fields["src"].Value,
		}
	}

	if cfg.Owner != "" && cfg.Group == "" {
		return nil, PartialOwnershipError{
			Set: "owner", Missing: "group",
			Source: step.Fields["owner"].Value,
		}
	}
	if cfg.Group != "" && cfg.Owner == "" {
		return nil, PartialOwnershipError{
			Set: "group", Missing: "owner",
			Source: step.Fields["group"].Value,
		}
	}

	var mode fs.FileMode
	if cfg.Perm != "" {
		var err error
		mode, err = perm.ParsePerm(cfg.Perm, step.Fields["perm"].Value)
		if err != nil {
			return nil, err
		}
	}

	return &unarchiveStep{
		desc:   cfg.Desc,
		kind:   u.Kind(),
		src:    cfg.Src.Path,
		srcRef: cfg.Src,
		dest:   cfg.Dest,
		depth:  cfg.Depth,
		owner:  cfg.Owner,
		group:  cfg.Group,
		mode:   mode,
		format: fmt,
		step:   step,
	}, nil
}

func (a *unarchiveStep) Desc() string { return a.desc }
func (a *unarchiveStep) Kind() string { return a.kind }
func (a *unarchiveStep) Inputs() []spec.Resource {
	var r []spec.Resource
	if a.owner != "" {
		r = append(r, spec.UserResource(a.owner))
	}
	if a.group != "" {
		r = append(r, spec.GroupResource(a.group))
	}
	return r
}
func (a *unarchiveStep) SourcePaths() []string { return []string{a.src} }
func (a *unarchiveStep) Promises() []spec.Resource {
	return []spec.Resource{spec.PathResource(a.dest)}
}

func (a *unarchiveStep) Ops() []spec.Op {
	extract := &unarchiveOp{
		BaseOp: sharedop.BaseOp{
			SrcSpan:  a.step.Fields["src"].Value,
			DestSpan: a.step.Fields["dest"].Value,
		},
		src:    a.src,
		srcRef: a.srcRef,
		dest:   a.dest,
		depth:  a.depth,
		format: a.format,
	}
	extract.SetStep(a)

	ops := []spec.Op{extract}
	ops = append(sharedop.ResolveSourceOps(a.srcRef, extract, a, a.step.Fields["src"].Value), ops...)

	if a.owner != "" && a.group != "" {
		chown := &fileop.EnsureOwnerOp{
			BaseOp: sharedop.BaseOp{
				DestSpan: a.step.Fields["dest"].Value,
			},
			Path:      a.dest,
			Owner:     a.owner,
			Group:     a.group,
			Recursive: true,
			OwnerSpan: a.step.Fields["owner"].Value,
			GroupSpan: a.step.Fields["group"].Value,
		}
		chown.SetStep(a)
		chown.AddDependency(extract)
		ops = append(ops, chown)
	}

	if a.mode != 0 {
		chmod := &fileop.EnsureModeOp{
			BaseOp: sharedop.BaseOp{
				DestSpan: a.step.Fields["dest"].Value,
			},
			Path:      a.dest,
			Mode:      a.mode,
			Recursive: true,
		}
		chmod.SetStep(a)
		chmod.AddDependency(extract)
		ops = append(ops, chmod)
	}

	return ops
}
