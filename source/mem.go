package source

import (
	"context"
	"io/fs"
	"sync"
	"time"
)

type MemSource struct {
	mu sync.RWMutex

	Files    map[string][]byte
	ModTimes map[string]time.Time
	Env      map[string]string
}

func NewMemSource() *MemSource {
	return &MemSource{
		Files:    make(map[string][]byte),
		ModTimes: make(map[string]time.Time),
		Env:      make(map[string]string),
	}
}

func (m *MemSource) ReadFile(_ context.Context, path string) ([]byte, error) {
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

func (m *MemSource) WriteFile(_ context.Context, path string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := make([]byte, len(data))
	copy(cp, data)
	m.Files[path] = cp
	m.ModTimes[path] = time.Now()
	return nil
}

func (m *MemSource) EnsureDir(_ context.Context, _ string) error {
	return nil
}

func (m *MemSource) Stat(_ context.Context, path string) (FileMeta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.Files[path]
	if !ok {
		return FileMeta{Exists: false}, nil
	}

	return FileMeta{
		Exists:   true,
		IsDir:    false,
		Size:     int64(len(data)),
		Modified: m.ModTimes[path],
	}, nil
}

func (m *MemSource) LookupEnv(key string) (string, bool) {
	v, ok := m.Env[key]
	return v, ok
}
