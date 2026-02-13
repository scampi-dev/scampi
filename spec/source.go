// SPDX-License-Identifier: GPL-3.0-only

package spec

import (
	"strings"
)

type SourceStore struct {
	files map[string][]string
}

func NewSourceStore() *SourceStore {
	return &SourceStore{
		files: make(map[string][]string),
	}
}

func (s *SourceStore) AddFile(name string, content string) {
	s.files[name] = splitLines(content)
}

func (s *SourceStore) Line(name string, line int) (string, bool) {
	if line <= 0 {
		return "", false
	}
	lines, ok := s.findFile(name)
	if !ok || line > len(lines) {
		return "", false
	}
	return lines[line-1], true
}

func (s *SourceStore) findFile(name string) ([]string, bool) {
	for _, p := range fallbackPaths(name) {
		if lines, ok := s.files[p]; ok {
			return lines, true
		}
	}

	return []string{}, false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
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
