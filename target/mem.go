package target

import (
	"context"
	"io/fs"
	"sync"
	"time"
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
		return nil, fs.ErrNotExist
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.Files[path]
	if !ok {
		return nil, fs.ErrNotExist
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
		return fs.ErrNotExist
	}

	m.Modes[path] = mode
	m.ModTimes[path] = time.Now()
	return nil
}

func (m *MemTarget) Chown(_ context.Context, path string, owner Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Files[path]; !ok {
		return fs.ErrNotExist
	}

	m.Owners[path] = owner
	return nil
}

func (m *MemTarget) GetOwner(_ context.Context, path string) (Owner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	owner, ok := m.Owners[path]
	if !ok {
		// mirror LocalPosixTarget behavior
		return Owner{}, nil
	}
	return owner, nil
}

type memFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (i memFileInfo) Name() string       { return i.name }
func (i memFileInfo) Size() int64        { return i.size }
func (i memFileInfo) Mode() fs.FileMode  { return i.mode }
func (i memFileInfo) ModTime() time.Time { return i.modTime }
func (i memFileInfo) IsDir() bool        { return false }
func (i memFileInfo) Sys() any           { return nil }
