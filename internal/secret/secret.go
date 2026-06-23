// SPDX-License-Identifier: GPL-3.0-only

// Package secret provides the Backend interface for resolving secret values
// and implementations of that interface.
package secret

import "sort"

// Backend resolves secret values by key.
type Backend interface {
	Name() string
	Lookup(key string) (string, bool, error)
	Keys() []string
}

// SortedKeys returns the sorted keys of a string map.
func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// PlaceholderBackend returns placeholder values for all known keys.
// Used by the LSP to continue evaluation without real secret access.
// Cause records why the real backend failed, so the LSP can surface it
// as a non-fatal hint.
type PlaceholderBackend struct {
	KeyMap map[string]string
	Cause  error
}

func (p *PlaceholderBackend) Name() string { return "placeholder" }

func (p *PlaceholderBackend) Lookup(key string) (string, bool, error) {
	if p.KeyMap == nil {
		return "<secret>", true, nil
	}
	v, ok := p.KeyMap[key]
	return v, ok, nil
}

func (p *PlaceholderBackend) Keys() []string {
	return SortedKeys(p.KeyMap)
}
