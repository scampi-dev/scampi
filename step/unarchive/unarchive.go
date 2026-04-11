// SPDX-License-Identifier: GPL-3.0-only

package unarchive

import (
	"io/fs"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/step/sharedops/fileops"
)

type (
	Unarchive       struct{}
	UnarchiveConfig struct {
		_ struct{} `summary:"Extract an archive to a target directory with optional recursive unpacking"`

		Desc  string         `step:"Human-readable description" optional:"true"`
		Src   spec.SourceRef `step:"Source archive" example:"local(\"./files/site.tar.gz\")"`
		Dest  string         `step:"Target directory for extraction" example:"/var/www/mysite"`
		Depth int            `step:"Nested archive recursion (-1=unlimited, 0=top only)" optional:"true" default:"0"`
		Owner string         `step:"Owner applied recursively after extraction" optional:"true" example:"www-data"`
		Group string         `step:"Group applied recursively after extraction" optional:"true" example:"www-data"`
		Perm  string         `step:"Permissions applied recursively after extraction" optional:"true" example:"0755"`
	}
	unarchiveAction struct {
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
		step   spec.StepInstance
	}
)

func (Unarchive) Kind() string   { return "unarchive" }
func (Unarchive) NewConfig() any { return &UnarchiveConfig{} }

func (u Unarchive) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*UnarchiveConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &UnarchiveConfig{}, step.Config)
	}

	// dest absoluteness and perm format are link-time checks via
	// @std.path(absolute=true) and @std.filemode on the stub.
	// Archive format detection (filename-based) and owner/group
	// mutual requirement are runtime/cross-field — they stay.
	fmt, ok := detectFormat(cfg.Src.Path)
	if !ok {
		return nil, UnsupportedArchiveError{
			Path:   cfg.Src.Path,
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
		mode, err = fileops.ParsePerm(cfg.Perm, step.Fields["perm"].Value)
		if err != nil {
			return nil, err
		}
	}

	return &unarchiveAction{
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

func (a *unarchiveAction) Desc() string { return a.desc }
func (a *unarchiveAction) Kind() string { return a.kind }
func (a *unarchiveAction) Inputs() []spec.Resource {
	var r []spec.Resource
	if a.owner != "" {
		r = append(r, spec.UserResource(a.owner))
	}
	if a.group != "" {
		r = append(r, spec.GroupResource(a.group))
	}
	return r
}
func (a *unarchiveAction) SourcePaths() []string { return []string{a.src} }
func (a *unarchiveAction) Promises() []spec.Resource {
	return []spec.Resource{spec.PathResource(a.dest)}
}

func (a *unarchiveAction) Ops() []spec.Op {
	extract := &unarchiveOp{
		BaseOp: sharedops.BaseOp{
			SrcSpan:  a.step.Fields["src"].Value,
			DestSpan: a.step.Fields["dest"].Value,
		},
		src:    a.src,
		srcRef: a.srcRef,
		dest:   a.dest,
		depth:  a.depth,
		format: a.format,
	}
	extract.SetAction(a)

	ops := []spec.Op{extract}
	ops = append(sharedops.ResolveSourceOps(a.srcRef, extract, a, a.step.Fields["src"].Value), ops...)

	if a.owner != "" && a.group != "" {
		chown := &fileops.EnsureOwnerOp{
			BaseOp: sharedops.BaseOp{
				DestSpan: a.step.Fields["dest"].Value,
			},
			Path:      a.dest,
			Owner:     a.owner,
			Group:     a.group,
			Recursive: true,
			OwnerSpan: a.step.Fields["owner"].Value,
			GroupSpan: a.step.Fields["group"].Value,
		}
		chown.SetAction(a)
		chown.AddDependency(extract)
		ops = append(ops, chown)
	}

	if a.mode != 0 {
		chmod := &fileops.EnsureModeOp{
			BaseOp: sharedops.BaseOp{
				DestSpan: a.step.Fields["dest"].Value,
			},
			Path:      a.dest,
			Mode:      a.mode,
			Recursive: true,
		}
		chmod.SetAction(a)
		chmod.AddDependency(extract)
		ops = append(ops, chmod)
	}

	return ops
}
