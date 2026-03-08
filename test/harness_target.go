// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"io/fs"
	"sync"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// faultySource wraps a source.Source and injects errors on configured paths.
type faultySource struct {
	source.Source

	mu     sync.RWMutex
	faults map[string]error
}

func newFaultySource(inner source.Source) *faultySource {
	return &faultySource{
		Source: inner,
		faults: make(map[string]error),
	}
}

func (f *faultySource) injectFault(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults[path] = &fakeDiagnostic{
		severity: signal.Error,
		impact:   diagnostic.ImpactAbort,
		cause:    err,
	}
}

//lint:ignore U1000 kept for symmetry with faultyTarget
func (f *faultySource) clearFaults() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults = make(map[string]error)
}

func (f *faultySource) getFault(path string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.faults[path]
}

func (f *faultySource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := f.getFault(path); err != nil {
		return nil, err
	}
	return f.Source.ReadFile(ctx, path)
}

func (f *faultySource) Stat(ctx context.Context, path string) (source.FileMeta, error) {
	if err := f.getFault(path); err != nil {
		return source.FileMeta{}, err
	}
	return f.Source.Stat(ctx, path)
}

// faultyTarget wraps a target.Target and injects errors on configured method/path pairs.
type faultyTarget struct {
	target.Target

	mu     sync.RWMutex
	faults map[faultKey]error
}

type faultKey struct {
	method string
	path   string
}

func newFaultyTarget(inner target.Target) *faultyTarget {
	return &faultyTarget{
		Target: inner,
		faults: make(map[faultKey]error),
	}
}

func (f *faultyTarget) injectFault(method, path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults[faultKey{method, path}] = &fakeDiagnostic{
		severity: signal.Error,
		impact:   diagnostic.ImpactAbort,
		cause:    err,
	}
}

func (f *faultyTarget) clearFaults() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults = make(map[faultKey]error)
}

func (f *faultyTarget) getFault(method, path string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.faults[faultKey{method, path}]
}

func (f *faultyTarget) Stat(ctx context.Context, path string) (fs.FileInfo, error) {
	if err := f.getFault("Stat", path); err != nil {
		return nil, err
	}
	return target.Must[target.Filesystem]("faultyTarget", f.Target).Stat(ctx, path)
}

func (f *faultyTarget) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := f.getFault("ReadFile", path); err != nil {
		return nil, err
	}
	return target.Must[target.Filesystem]("faultyTarget", f.Target).ReadFile(ctx, path)
}

func (f *faultyTarget) WriteFile(ctx context.Context, path string, data []byte) error {
	if err := f.getFault("WriteFile", path); err != nil {
		return err
	}
	return target.Must[target.Filesystem]("faultyTarget", f.Target).WriteFile(ctx, path, data)
}

func (f *faultyTarget) Remove(ctx context.Context, path string) error {
	if err := f.getFault("Remove", path); err != nil {
		return err
	}
	return target.Must[target.Filesystem]("faultyTarget", f.Target).Remove(ctx, path)
}

func (f *faultyTarget) Mkdir(ctx context.Context, path string, mode fs.FileMode) error {
	if err := f.getFault("Mkdir", path); err != nil {
		return err
	}
	return target.Must[target.Filesystem]("faultyTarget", f.Target).Mkdir(ctx, path, mode)
}

func (f *faultyTarget) Chmod(ctx context.Context, path string, mode fs.FileMode) error {
	if err := f.getFault("Chmod", path); err != nil {
		return err
	}
	return target.Must[target.FileMode]("faultyTarget", f.Target).Chmod(ctx, path, mode)
}

func (f *faultyTarget) Chown(ctx context.Context, path string, owner target.Owner) error {
	if err := f.getFault("Chown", path); err != nil {
		return err
	}
	return target.Must[target.Ownership]("faultyTarget", f.Target).Chown(ctx, path, owner)
}

func (f *faultyTarget) GetOwner(ctx context.Context, path string) (target.Owner, error) {
	if err := f.getFault("GetOwner", path); err != nil {
		return target.Owner{}, err
	}
	return target.Must[target.Ownership]("faultyTarget", f.Target).GetOwner(ctx, path)
}

func (f *faultyTarget) HasUser(ctx context.Context, user string) bool {
	return target.Must[target.Ownership]("faultyTarget", f.Target).HasUser(ctx, user)
}

func (f *faultyTarget) HasGroup(ctx context.Context, group string) bool {
	return target.Must[target.Ownership]("faultyTarget", f.Target).HasGroup(ctx, group)
}

type minimalTarget struct {
	*target.MemTarget
}

func newMinimalTarget() *minimalTarget {
	return &minimalTarget{MemTarget: target.NewMemTarget()}
}

func (m *minimalTarget) Capabilities() capability.Capability {
	return capability.Filesystem
}

func (m *minimalTarget) HasUser(_ context.Context, _ string) bool {
	panic("MinimalTarget.HasUser called - capability check failed")
}

func (m *minimalTarget) HasGroup(_ context.Context, _ string) bool {
	panic("MinimalTarget.HasGroup called - capability check failed")
}

func (m *minimalTarget) GetOwner(_ context.Context, _ string) (target.Owner, error) {
	panic("MinimalTarget.GetOwner called - capability check failed")
}

// pkgOnlyTarget advertises Pkg but not PkgUpdate.
type pkgOnlyTarget struct {
	*target.MemTarget
}

func newPkgOnlyTarget() *pkgOnlyTarget {
	return &pkgOnlyTarget{MemTarget: target.NewMemTarget()}
}

func (p *pkgOnlyTarget) Capabilities() capability.Capability {
	return capability.POSIX | capability.Pkg
}

func (p *pkgOnlyTarget) UpdateCache(_ context.Context) error {
	panic("pkgOnlyTarget.UpdateCache called — capability check failed")
}

func (p *pkgOnlyTarget) IsUpgradable(_ context.Context, _ string) (bool, error) {
	panic("pkgOnlyTarget.IsUpgradable called — capability check failed")
}

type allCapNoImplTarget struct{}

func (allCapNoImplTarget) Capabilities() capability.Capability {
	return capability.All
}

type mockTargetType struct {
	tgt target.Target
}

func (mockTargetType) Kind() string   { return "mem" }
func (mockTargetType) NewConfig() any { return nil }
func (t mockTargetType) Create(_ context.Context, _ source.Source, _ spec.TargetInstance) (target.Target, error) {
	return t.tgt, nil
}

func mockTargetInstance(tgt target.Target) spec.TargetInstance {
	return spec.TargetInstance{
		Type: mockTargetType{
			tgt: tgt,
		},
	}
}
