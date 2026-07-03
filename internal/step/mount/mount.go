// SPDX-License-Identifier: GPL-3.0-only

// Package mount implements the mount step for managing filesystem mounts.
package mount

import (
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
)

// FsType represents a supported filesystem type.
type FsType uint8

const (
	FsNFS FsType = iota + 1
	FsNFS4
	FsCIFS
	FsExt4
	FsXFS
	FsBtrfs
	FsTmpfs
	FsGlusterfs
	FsCeph
)

const (
	fsNFS       = "nfs"
	fsNFS4      = "nfs4"
	fsCIFS      = "cifs"
	fsExt4      = "ext4"
	fsXFS       = "xfs"
	fsBtrfs     = "btrfs"
	fsTmpfs     = "tmpfs"
	fsGlusterfs = "glusterfs"
	fsCeph      = "ceph"
)

func (f FsType) String() string {
	switch f {
	case FsNFS:
		return fsNFS
	case FsNFS4:
		return fsNFS4
	case FsCIFS:
		return fsCIFS
	case FsExt4:
		return fsExt4
	case FsXFS:
		return fsXFS
	case FsBtrfs:
		return fsBtrfs
	case FsTmpfs:
		return fsTmpfs
	case FsGlusterfs:
		return fsGlusterfs
	case FsCeph:
		return fsCeph
	default:
		return "unknown"
	}
}

func parseFsType(s string) FsType {
	switch s {
	case fsNFS:
		return FsNFS
	case fsNFS4:
		return FsNFS4
	case fsCIFS:
		return FsCIFS
	case fsExt4:
		return FsExt4
	case fsXFS:
		return FsXFS
	case fsBtrfs:
		return FsBtrfs
	case fsTmpfs:
		return FsTmpfs
	case fsGlusterfs:
		return FsGlusterfs
	case fsCeph:
		return FsCeph
	default:
		panic(errs.BUG("invalid mount fs_type %q - should have been caught by lang typechecker", s))
	}
}

// NeedsHelper reports whether this filesystem type requires a userspace
// mount helper (e.g. mount.nfs, mount.cifs).
func (f FsType) NeedsHelper() bool {
	return f.HelperBinary() != ""
}

// HelperBinary returns the mount helper binary name, or empty if none needed.
func (f FsType) HelperBinary() string {
	switch f {
	case FsNFS, FsNFS4:
		return "mount.nfs"
	case FsCIFS:
		return "mount.cifs"
	case FsGlusterfs:
		return "mount.glusterfs"
	case FsCeph:
		return "mount.ceph"
	default:
		return ""
	}
}

// State represents the desired mount state.
type State uint8

const (
	StateMounted State = iota + 1
	StateUnmounted
	StateAbsent
)

const (
	stateMounted   = "mounted"
	stateUnmounted = "unmounted"
	stateAbsent    = "absent"
)

func (s State) String() string {
	switch s {
	case StateMounted:
		return stateMounted
	case StateUnmounted:
		return stateUnmounted
	case StateAbsent:
		return stateAbsent
	default:
		return "unknown"
	}
}

type (
	Mount       struct{}
	MountConfig struct {
		Desc     string
		Src      string
		Dest     string
		Type     string
		Opts     string
		State    string
		Provides []string
		Requires []string
	}
	mountStep struct {
		desc  string
		src   string
		dest  string
		fstyp FsType
		opts  string
		state State
		step  spec.DeclaredStep
	}
)

func (Mount) Kind() string   { return "mount" }
func (Mount) NewConfig() any { return &MountConfig{} }

func (c *MountConfig) ResourceDeclarations() (provides, requires []string) {
	return c.Provides, c.Requires
}

func (Mount) Plan(step spec.DeclaredStep) (spec.Step, error) {
	cfg, ok := step.Config.(*MountConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &MountConfig{}, step.Config)
	}

	// Field shapes are validated at link time:
	//   - src: @std.nonempty
	//   - dest: @std.path(absolute=true) (catches both empty and relative)
	//   - fs_type, state: typed enums in the stub (lang/check enforces)
	//   - opts, state defaults: declared in the stub, materialized by eval
	// Only the typed-enum conversions remain.
	fstyp := parseFsType(cfg.Type)

	var state State
	switch cfg.State {
	case stateMounted:
		state = StateMounted
	case stateUnmounted:
		state = StateUnmounted
	case stateAbsent:
		state = StateAbsent
	default:
		panic(errs.BUG("invalid mount state %q - should have been caught by lang typechecker", cfg.State))
	}

	return &mountStep{
		desc:  cfg.Desc,
		src:   cfg.Src,
		dest:  cfg.Dest,
		fstyp: fstyp,
		opts:  cfg.Opts,
		state: state,
		step:  step,
	}, nil
}

func (a *mountStep) Desc() string { return a.desc }
func (a *mountStep) Kind() string { return "mount" }
func (a *mountStep) Ops() []spec.Op {
	op := &ensureMountOp{
		src:   a.src,
		dest:  a.dest,
		fstyp: a.fstyp,
		opts:  a.opts,
		state: a.state,
	}
	op.SetStep(a)
	return []spec.Op{op}
}
