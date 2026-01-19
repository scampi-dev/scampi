package target

import (
	"context"
	"io/fs"
	"sync"
	"time"

	"godoit.dev/doit/util"
)

type MemTarget struct {
	mu sync.RWMutex

	Files    map[string][]byte
	Modes    map[string]fs.FileMode
	Owners   map[string]Owner
	ModTimes map[string]time.Time
}

func NewMemTarget() *MemTarget {
	return &MemTarget{
		Files:    make(map[string][]byte),
		Modes:    make(map[string]fs.FileMode),
		Owners:   make(map[string]Owner),
		ModTimes: make(map[string]time.Time),
	}
}

func (m *MemTarget) ReadFile(_ context.Context, path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.Files[path]
	if !ok {
		return nil, util.WrapErrf(ErrNotExist, "%q", path)
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
		return nil, util.WrapErrf(ErrNotExist, "%q", path)
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
		return util.WrapErrf(ErrNotExist, "%q", path)
	}

	m.Modes[path] = mode
	m.ModTimes[path] = time.Now()
	return nil
}

func (m *MemTarget) Chown(_ context.Context, path string, owner Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Files[path]; !ok {
		return util.WrapErrf(ErrNotExist, "%q", path)
	}

	m.Owners[path] = owner
	return nil
}

func (m *MemTarget) GetOwner(_ context.Context, path string) (Owner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.Files[path]; !ok {
		return Owner{}, util.WrapErrf(ErrNotExist, "%q", path)
	}

	return m.Owners[path], nil
}

type memFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (i memFileInfo) Name() string       { return i.name }
func (i memFileInfo) Size() int64        { return i.size }
func (i memFileInfo) Mode() fs.FileMode  { return i.mode }
func (i memFileInfo) ModTime() time.Time { return i.modTime }
func (i memFileInfo) IsDir() bool        { return i.isDir }
func (i memFileInfo) Sys() any           { return nil }
