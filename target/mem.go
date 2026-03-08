// SPDX-License-Identifier: GPL-3.0-only

package target

import (
	"context"
	"io/fs"
	"sync"
	"time"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
)

type commandCall struct {
	Cmd string
}

type MemTarget struct {
	mu sync.RWMutex

	Files      map[string][]byte
	Dirs       map[string]fs.FileMode
	Modes      map[string]fs.FileMode
	Owners     map[string]Owner
	ModTimes   map[string]time.Time
	Symlinks   map[string]string
	Pkgs       map[string]bool
	Upgradable map[string]bool

	Services        map[string]bool // service name -> active (running)
	EnabledServices map[string]bool // service name -> enabled at boot

	Commands    []commandCall
	CommandFunc func(cmd string) (CommandResult, error)
}

func NewMemTarget() *MemTarget {
	return &MemTarget{
		Files:           make(map[string][]byte),
		Dirs:            make(map[string]fs.FileMode),
		Modes:           make(map[string]fs.FileMode),
		Owners:          make(map[string]Owner),
		ModTimes:        make(map[string]time.Time),
		Symlinks:        make(map[string]string),
		Pkgs:            make(map[string]bool),
		Upgradable:      make(map[string]bool),
		Services:        make(map[string]bool),
		EnabledServices: make(map[string]bool),
	}
}

func (m *MemTarget) ReadFile(_ context.Context, path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.Files[path]
	if !ok {
		return nil, errs.WrapErrf(ErrNotExist, "%q", path)
	}

	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func (m *MemTarget) WriteFile(_ context.Context, path string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := make([]byte, len(data))
	copy(cp, data)

	m.Files[path] = cp
	m.Modes[path] = 0o644
	m.ModTimes[path] = time.Now()
	// Set default owner so chown to the same owner is a no-op (matches SSH behavior)
	if _, exists := m.Owners[path]; !exists {
		m.Owners[path] = Owner{User: "testuser", Group: "testgroup"}
	}
	return nil
}

// isImplicitDir reports whether path is a parent of any file or symlink.
// Caller must hold mu.RLock.
func (m *MemTarget) isImplicitDir(path string) bool {
	dirPrefix := path + "/"
	for p := range m.Files {
		if len(p) > len(dirPrefix) && p[:len(dirPrefix)] == dirPrefix {
			return true
		}
	}
	for p := range m.Symlinks {
		if len(p) > len(dirPrefix) && p[:len(dirPrefix)] == dirPrefix {
			return true
		}
	}
	return false
}

func (m *MemTarget) Stat(_ context.Context, path string) (fs.FileInfo, error) {
	if path == "/" {
		return memFileInfo{
			name:  "/",
			mode:  fs.ModeDir | 0o755,
			isDir: true,
		}, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.Files[path]
	if ok {
		return memFileInfo{
			name:    path,
			size:    int64(len(data)),
			mode:    m.Modes[path],
			modTime: m.ModTimes[path],
		}, nil
	}

	if dirMode, ok := m.Dirs[path]; ok {
		return memFileInfo{
			name:  path,
			mode:  fs.ModeDir | dirMode,
			isDir: true,
		}, nil
	}

	if m.isImplicitDir(path) {
		return memFileInfo{
			name:  path,
			mode:  fs.ModeDir | 0o755,
			isDir: true,
		}, nil
	}

	return nil, errs.WrapErrf(ErrNotExist, "%q", path)
}

func (m *MemTarget) Mkdir(_ context.Context, path string, mode fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Dirs[path] = mode
	m.Modes[path] = mode
	if _, exists := m.Owners[path]; !exists {
		m.Owners[path] = Owner{User: "testuser", Group: "testgroup"}
	}
	return nil
}

func (m *MemTarget) Chmod(_ context.Context, path string, mode fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, isFile := m.Files[path]
	_, isDir := m.Dirs[path]
	if !isFile && !isDir {
		return errs.WrapErrf(ErrNotExist, "%q", path)
	}

	m.Modes[path] = mode
	m.ModTimes[path] = time.Now()
	if isDir {
		m.Dirs[path] = mode
	}
	return nil
}

func (m *MemTarget) Chown(_ context.Context, path string, owner Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, isFile := m.Files[path]
	_, isDir := m.Dirs[path]
	if !isFile && !isDir {
		return errs.WrapErrf(ErrNotExist, "%q", path)
	}

	m.Owners[path] = owner
	return nil
}

func (m *MemTarget) HasUser(_ context.Context, _ string) bool {
	return true
}

func (m *MemTarget) HasGroup(_ context.Context, _ string) bool {
	return true
}

func (m *MemTarget) GetOwner(_ context.Context, path string) (Owner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, isFile := m.Files[path]
	_, isDir := m.Dirs[path]
	if !isFile && !isDir {
		return Owner{}, errs.WrapErrf(ErrNotExist, "%q", path)
	}

	return m.Owners[path], nil
}

func (m *MemTarget) Lstat(_ context.Context, path string) (fs.FileInfo, error) {
	if path == "/" {
		return memFileInfo{
			name:  "/",
			mode:  fs.ModeDir | 0o755,
			isDir: true,
		}, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if target, ok := m.Symlinks[path]; ok {
		return memFileInfo{
			name: path,
			mode: fs.ModeSymlink | 0o777,
			sys:  target,
		}, nil
	}

	data, ok := m.Files[path]
	if ok {
		return memFileInfo{
			name:    path,
			size:    int64(len(data)),
			mode:    m.Modes[path],
			modTime: m.ModTimes[path],
		}, nil
	}

	if m.isImplicitDir(path) {
		return memFileInfo{
			name:  path,
			mode:  fs.ModeDir | 0o755,
			isDir: true,
		}, nil
	}

	return nil, errs.WrapErrf(ErrNotExist, "%q", path)
}

func (m *MemTarget) Readlink(_ context.Context, path string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	target, ok := m.Symlinks[path]
	if !ok {
		return "", errs.WrapErrf(ErrNotExist, "%q", path)
	}

	return target, nil
}

func (m *MemTarget) Symlink(_ context.Context, target, link string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Symlinks[link] = target
	return nil
}

func (m *MemTarget) Remove(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check symlinks first
	if _, ok := m.Symlinks[path]; ok {
		delete(m.Symlinks, path)
		return nil
	}

	// Check regular files
	if _, ok := m.Files[path]; ok {
		delete(m.Files, path)
		delete(m.Modes, path)
		delete(m.Owners, path)
		delete(m.ModTimes, path)
		return nil
	}

	return errs.WrapErrf(ErrNotExist, "%q", path)
}

func (m *MemTarget) Capabilities() capability.Capability {
	return capability.POSIX | capability.Pkg | capability.PkgUpdate | capability.Service
}

func (m *MemTarget) IsInstalled(_ context.Context, pkg string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Pkgs[pkg], nil
}

func (m *MemTarget) InstallPkgs(_ context.Context, pkgs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, pkg := range pkgs {
		m.Pkgs[pkg] = true
	}
	return nil
}

func (m *MemTarget) RemovePkgs(_ context.Context, pkgs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, pkg := range pkgs {
		delete(m.Pkgs, pkg)
	}
	return nil
}

func (m *MemTarget) UpdateCache(_ context.Context) error {
	return nil
}

func (m *MemTarget) IsUpgradable(_ context.Context, pkg string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Upgradable[pkg], nil
}

func (m *MemTarget) IsActive(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Services[name], nil
}

func (m *MemTarget) IsEnabled(_ context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.EnabledServices[name], nil
}

func (m *MemTarget) Start(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Services[name] = true
	return nil
}

func (m *MemTarget) Stop(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Services[name] = false
	return nil
}

func (m *MemTarget) Enable(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnabledServices[name] = true
	return nil
}

func (m *MemTarget) Disable(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnabledServices[name] = false
	return nil
}

func (m *MemTarget) DaemonReload(_ context.Context) error {
	return nil
}

func (m *MemTarget) RunCommand(_ context.Context, cmd string) (CommandResult, error) {
	m.mu.Lock()
	m.Commands = append(m.Commands, commandCall{Cmd: cmd})
	fn := m.CommandFunc
	m.mu.Unlock()

	if fn != nil {
		return fn(cmd)
	}
	return CommandResult{ExitCode: 127, Stderr: "command not found"}, nil
}

type memFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
	sys     any
}

func (i memFileInfo) Name() string       { return i.name }
func (i memFileInfo) Size() int64        { return i.size }
func (i memFileInfo) Mode() fs.FileMode  { return i.mode }
func (i memFileInfo) ModTime() time.Time { return i.modTime }
func (i memFileInfo) IsDir() bool        { return i.isDir }
func (i memFileInfo) Sys() any           { return i.sys }
