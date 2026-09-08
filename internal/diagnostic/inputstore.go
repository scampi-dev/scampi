// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"strings"
	"sync"
)

// InputStore caches the bytes of user-authored input files - the .scampi
// config plus every file it references (templates, copy sources) - so the
// renderer can quote the offending line under a diagnostic.
//
// It holds input only, never target state. Read via Line; nothing else
// needs the raw bytes.
//
// InputStore is safe for concurrent use - multiple plan workers register
// files in parallel while the renderer reads lines.
type InputStore struct {
	mu    sync.RWMutex
	files map[string][]byte
}

func NewInputStore() *InputStore {
	return &InputStore{
		files: make(map[string][]byte),
	}
}

func (s *InputStore) AddFile(name string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[name] = data
}

func (s *InputStore) Line(name string, line int) (string, bool) {
	if line <= 0 {
		return "", false
	}
	data, ok := s.findFile(name)
	if !ok {
		return "", false
	}
	// Scan raw bytes to find the Nth line on demand.
	n := 1
	start := 0
	for i, b := range data {
		if b == '\n' {
			if n == line {
				return string(data[start:i]), true
			}
			n++
			start = i + 1
		}
	}
	if n == line {
		return string(data[start:]), true
	}
	return "", false
}

func (s *InputStore) findFile(name string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range fallbackPaths(name) {
		if data, ok := s.files[p]; ok {
			return data, true
		}
	}
	return nil, false
}

func fallbackPaths(path string) []string {
	trimmed := strings.Trim(path, "/")

	var result []string
	if trimmed != path {
		result = append(result, path)
	}

	for {
		result = append(result, trimmed)

		idx := strings.Index(trimmed, "/")
		if idx == -1 {
			break
		}

		trimmed = trimmed[idx+1:]
	}

	return result
}
