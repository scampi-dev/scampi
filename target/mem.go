package target

import (
	"context"
	"io/fs"
	"sync"
	"time"

	"godoit.dev/doit/errs"
)

type MemTarget struct {
	mu sync.RWMutex

	Files    map[string][]byte
	Modes    map[string]fs.FileMode
	Owners   map[string]Owner
	ModTimes map[string]time.Time
	Symlinks map[string]string
}

func NewMemTarget() *MemTarget {
	return &MemTarget{
		Files:    make(map[string][]byte),
		Modes:    make(map[string]fs.FileMode),
		Owners:   make(map[string]Owner),
		ModTimes: make(map[string]time.Time),
		Symlinks: make(map[string]string),
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

func (m *MemTarget) WriteFile(_ context.Context, path string, data []byte, perm fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := make([]byte, len(data))
	copy(cp, data)

	m.Files[path] = cp
	m.Modes[path] = perm
	m.ModTimes[path] = time.Now()
	return nil
}

func (m *MemTarget) Stat(_ context.Context, path string) (fs.FileInfo, error) {
	// Root directory always exists
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
	if !ok {
		return nil, errs.WrapErrf(ErrNotExist, "%q", path)
	}

	mode := m.Modes[path]
	mod := m.ModTimes[path]

	return memFileInfo{
		name:    path,
		size:    int64(len(data)),
		mode:    mode,
		modTime: mod,
	}, nil
}

func (m *MemTarget) Chmod(_ context.Context, path string, mode fs.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Files[path]; !ok {
		return errs.WrapErrf(ErrNotExist, "%q", path)
	}

	m.Modes[path] = mode
	m.ModTimes[path] = time.Now()
	return nil
}

func (m *MemTarget) Chown(_ context.Context, path string, owner Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Files[path]; !ok {
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

	if _, ok := m.Files[path]; !ok {
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

	// Check if it's a symlink first
	if target, ok := m.Symlinks[path]; ok {
		return memFileInfo{
			name: path,
			mode: fs.ModeSymlink | 0o777,
			sys:  target,
		}, nil
	}

	// Fall back to regular file
	data, ok := m.Files[path]
	if !ok {
		return nil, errs.WrapErrf(ErrNotExist, "%q", path)
	}

	mode := m.Modes[path]
	mod := m.ModTimes[path]

	return memFileInfo{
		name:    path,
		size:    int64(len(data)),
		mode:    mode,
		modTime: mod,
	}, nil
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
