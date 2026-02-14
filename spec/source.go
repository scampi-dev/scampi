// SPDX-License-Identifier: GPL-3.0-only

package spec

import (
	"strings"
)

type SourceStore struct {
	files map[string][]byte
}

func NewSourceStore() *SourceStore {
	return &SourceStore{
		files: make(map[string][]byte),
	}
}

func (s *SourceStore) AddFile(name string, data []byte) {
	s.files[name] = data
}

func (s *SourceStore) Line(name string, line int) (string, bool) {
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

func (s *SourceStore) findFile(name string) ([]byte, bool) {
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
