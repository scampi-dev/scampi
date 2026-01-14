package source

import (
	"context"
	"io/fs"
	"time"
)

type MemSource struct {
	Files    map[string][]byte
	ModTimes map[string]time.Time
}

func NewMemSource() *MemSource {
	return &MemSource{
		Files:    make(map[string][]byte),
		ModTimes: make(map[string]time.Time),
	}
}

func (m *MemSource) ReadFile(_ context.Context, path string) ([]byte, error) {
	data, ok := m.Files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}

	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func (m *MemSource) WriteFile(_ context.Context, path string, data []byte) error {
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
	data, ok := m.Files[path]
	if !ok {
		return FileMeta{Exists: false}, nil
	}

	return FileMeta{
		Exists:   true,
		Size:     int64(len(data)),
		Modified: m.ModTimes[path],
	}, nil
}
