// SPDX-License-Identifier: GPL-3.0-only

package harness

import (
	"context"
	"io/fs"
	"sync"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/signal"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

// FaultySource wraps a source.Source and injects errors on configured paths.
type FaultySource struct {
	source.Source

	mu     sync.RWMutex
	faults map[string]error
}

func NewFaultySource(inner source.Source) *FaultySource {
	return &FaultySource{
		Source: inner,
		faults: make(map[string]error),
	}
}

func (f *FaultySource) InjectFault(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults[path] = &FakeDiagnostic{
		severity: signal.Error,
		impact:   diagnostic.ImpactAbort,
		cause:    err,
	}
}

//lint:ignore U1000 kept for symmetry with FaultyTarget
func (f *FaultySource) ClearFaults() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults = make(map[string]error)
}

func (f *FaultySource) getFault(path string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.faults[path]
}

func (f *FaultySource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := f.getFault(path); err != nil {
		return nil, err
	}
	return f.Source.ReadFile(ctx, path)
}

func (f *FaultySource) Stat(ctx context.Context, path string) (source.FileMeta, error) {
	if err := f.getFault(path); err != nil {
		return source.FileMeta{}, err
	}
	return f.Source.Stat(ctx, path)
}

// FaultyTarget wraps a target.Target and injects errors on configured method/path pairs.
type FaultyTarget struct {
	target.Target

	mu     sync.RWMutex
	faults map[faultKey]error
}

type faultKey struct {
	method string
	path   string
}

func NewFaultyTarget(inner target.Target) *FaultyTarget {
	return &FaultyTarget{
		Target: inner,
		faults: make(map[faultKey]error),
	}
}

func (f *FaultyTarget) InjectFault(method, path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults[faultKey{method, path}] = &FakeDiagnostic{
		severity: signal.Error,
		impact:   diagnostic.ImpactAbort,
		cause:    err,
	}
}

func (f *FaultyTarget) ClearFaults() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults = make(map[faultKey]error)
}

func (f *FaultyTarget) getFault(method, path string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.faults[faultKey{method, path}]
}

func (f *FaultyTarget) Stat(ctx context.Context, path string) (fs.FileInfo, error) {
	if err := f.getFault("Stat", path); err != nil {
		return nil, err
	}
	return target.Must[target.Filesystem]("FaultyTarget", f.Target).Stat(ctx, path)
}

func (f *FaultyTarget) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := f.getFault("ReadFile", path); err != nil {
		return nil, err
	}
	return target.Must[target.Filesystem]("FaultyTarget", f.Target).ReadFile(ctx, path)
}

func (f *FaultyTarget) WriteFile(ctx context.Context, path string, data []byte) error {
	if err := f.getFault("WriteFile", path); err != nil {
		return err
	}
	return target.Must[target.Filesystem]("FaultyTarget", f.Target).WriteFile(ctx, path, data)
}

func (f *FaultyTarget) Remove(ctx context.Context, path string) error {
	if err := f.getFault("Remove", path); err != nil {
		return err
	}
	return target.Must[target.Filesystem]("FaultyTarget", f.Target).Remove(ctx, path)
}

func (f *FaultyTarget) Mkdir(ctx context.Context, path string, mode fs.FileMode) error {
	if err := f.getFault("Mkdir", path); err != nil {
		return err
	}
	return target.Must[target.Filesystem]("FaultyTarget", f.Target).Mkdir(ctx, path, mode)
}

func (f *FaultyTarget) ReadDir(ctx context.Context, path string) ([]fs.DirEntry, error) {
	if err := f.getFault("ReadDir", path); err != nil {
		return nil, err
	}
	return target.Must[target.Filesystem]("FaultyTarget", f.Target).ReadDir(ctx, path)
}

func (f *FaultyTarget) Chmod(ctx context.Context, path string, mode fs.FileMode) error {
	if err := f.getFault("Chmod", path); err != nil {
		return err
	}
	return target.Must[target.FileMode]("FaultyTarget", f.Target).Chmod(ctx, path, mode)
}

func (f *FaultyTarget) Chown(ctx context.Context, path string, owner target.Owner) error {
	if err := f.getFault("Chown", path); err != nil {
		return err
	}
	return target.Must[target.Ownership]("FaultyTarget", f.Target).Chown(ctx, path, owner)
}

func (f *FaultyTarget) ChownRecursive(ctx context.Context, path string, owner target.Owner) error {
	if err := f.getFault("ChownRecursive", path); err != nil {
		return err
	}
	return target.Must[target.Ownership]("FaultyTarget", f.Target).ChownRecursive(ctx, path, owner)
}

func (f *FaultyTarget) ChmodRecursive(ctx context.Context, path string, mode fs.FileMode) error {
	if err := f.getFault("ChmodRecursive", path); err != nil {
		return err
	}
	return target.Must[target.FileMode]("FaultyTarget", f.Target).ChmodRecursive(ctx, path, mode)
}

func (f *FaultyTarget) GetOwner(ctx context.Context, path string) (target.Owner, error) {
	if err := f.getFault("GetOwner", path); err != nil {
		return target.Owner{}, err
	}
	return target.Must[target.Ownership]("FaultyTarget", f.Target).GetOwner(ctx, path)
}

func (f *FaultyTarget) HasUser(ctx context.Context, user string) bool {
	return target.Must[target.Ownership]("FaultyTarget", f.Target).HasUser(ctx, user)
}

func (f *FaultyTarget) HasGroup(ctx context.Context, group string) bool {
	return target.Must[target.Ownership]("FaultyTarget", f.Target).HasGroup(ctx, group)
}

type MinimalTarget struct {
	*target.MemTarget
}

func NewMinimalTarget() *MinimalTarget {
	return &MinimalTarget{MemTarget: target.NewMemTarget()}
}

func (m *MinimalTarget) Capabilities() capability.Capability {
	return capability.Filesystem
}

func (m *MinimalTarget) HasUser(_ context.Context, _ string) bool {
	panic("MinimalTarget.HasUser called - capability check failed")
}

func (m *MinimalTarget) HasGroup(_ context.Context, _ string) bool {
	panic("MinimalTarget.HasGroup called - capability check failed")
}

func (m *MinimalTarget) GetOwner(_ context.Context, _ string) (target.Owner, error) {
	panic("MinimalTarget.GetOwner called - capability check failed")
}

// PkgOnlyTarget advertises Pkg but not PkgUpdate.
type PkgOnlyTarget struct {
	*target.MemTarget
}

func NewPkgOnlyTarget() *PkgOnlyTarget {
	return &PkgOnlyTarget{MemTarget: target.NewMemTarget()}
}

func (p *PkgOnlyTarget) Capabilities() capability.Capability {
	return capability.POSIX | capability.Pkg
}

func (p *PkgOnlyTarget) UpdateCache(_ context.Context) error {
	panic("PkgOnlyTarget.UpdateCache called — capability check failed")
}

func (p *PkgOnlyTarget) IsUpgradable(_ context.Context, _ string) (bool, error) {
	panic("PkgOnlyTarget.IsUpgradable called — capability check failed")
}

// SymlinkOnlyTarget advertises Symlink but not Filesystem.
type SymlinkOnlyTarget struct {
	*target.MemTarget
}

func NewSymlinkOnlyTarget() *SymlinkOnlyTarget {
	return &SymlinkOnlyTarget{MemTarget: target.NewMemTarget()}
}

func (s *SymlinkOnlyTarget) Capabilities() capability.Capability {
	return capability.Symlink
}

func (s *SymlinkOnlyTarget) Stat(_ context.Context, _ string) (fs.FileInfo, error) {
	panic("SymlinkOnlyTarget.Stat called — capability check failed")
}

func (s *SymlinkOnlyTarget) ReadFile(_ context.Context, _ string) ([]byte, error) {
	panic("SymlinkOnlyTarget.ReadFile called — capability check failed")
}

func (s *SymlinkOnlyTarget) WriteFile(_ context.Context, _ string, _ []byte) error {
	panic("SymlinkOnlyTarget.WriteFile called — capability check failed")
}

func (s *SymlinkOnlyTarget) Remove(_ context.Context, _ string) error {
	panic("SymlinkOnlyTarget.Remove called — capability check failed")
}

func (s *SymlinkOnlyTarget) Mkdir(_ context.Context, _ string, _ fs.FileMode) error {
	panic("SymlinkOnlyTarget.Mkdir called — capability check failed")
}

// NoCommandTarget advertises Filesystem but not Command.
type NoCommandTarget struct {
	*target.MemTarget
}

func NewNoCommandTarget() *NoCommandTarget {
	return &NoCommandTarget{MemTarget: target.NewMemTarget()}
}

func (n *NoCommandTarget) Capabilities() capability.Capability {
	return capability.Filesystem
}

func (n *NoCommandTarget) RunCommand(_ context.Context, _ string) (target.CommandResult, error) {
	panic("NoCommandTarget.RunCommand called — capability check failed")
}

type mockTargetKind struct {
	tgt target.Target
}

func (mockTargetKind) Kind() string   { return "mem" }
func (mockTargetKind) NewConfig() any { return nil }
func (t mockTargetKind) Create(_ context.Context, _ source.Source, _ spec.DeclaredTarget) (target.Target, error) {
	return t.tgt, nil
}

func MockDeclaredTarget(tgt target.Target) spec.DeclaredTarget {
	return spec.DeclaredTarget{
		Type: mockTargetKind{
			tgt: tgt,
		},
	}
}
