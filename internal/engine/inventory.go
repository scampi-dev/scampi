// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"maps"
	"slices"
	"strings"
)

// Inventory tracks the refs scampi manages and the data needed to
// destroy them. Not safe for concurrent use.
type Inventory struct {
	entries map[Ref]inventoryEntry
}

type inventoryEntry struct {
	attrs Attrs
	deps  []Ref
}

func NewInventory() *Inventory {
	return &Inventory{entries: map[Ref]inventoryEntry{}}
}

func (i *Inventory) Add(ref Ref, attrs Attrs, deps []Ref) {
	i.entries[ref] = inventoryEntry{
		attrs: maps.Clone(attrs),
		deps:  slices.Clone(deps),
	}
}

func (i *Inventory) Remove(ref Ref) { delete(i.entries, ref) }

func (i *Inventory) Has(ref Ref) bool {
	_, ok := i.entries[ref]
	return ok
}

// Get returns defensive copies of the stored attrs and deps.
func (i *Inventory) Get(ref Ref) (attrs Attrs, deps []Ref, ok bool) {
	e, ok := i.entries[ref]
	if !ok {
		return nil, nil, false
	}
	return maps.Clone(e.attrs), slices.Clone(e.deps), true
}

// Orphans returns refs in the inventory but not in declared.
func (i *Inventory) Orphans(declared []Resource) []Ref {
	declaredSet := make(map[Ref]bool, len(declared))
	for _, r := range declared {
		declaredSet[r.Ref()] = true
	}
	var orphans []Ref
	for ref := range i.entries {
		if !declaredSet[ref] {
			orphans = append(orphans, ref)
		}
	}
	return orphans
}

// Fold integrates one event. Unknown codes are ignored so the
// projection survives new code additions.
func (i *Inventory) Fold(code Code, ref Ref, attrs Attrs) {
	switch code {
	case CodeApplySuccess:
		deps := parseDeps(attrs["deps"])
		delete(attrs, "deps")
		i.Add(ref, attrs, deps)
	case CodeDestroySuccess:
		i.Remove(ref)
	}
}

func parseDeps(s string) []Ref {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]Ref, 0, len(parts))
	for _, p := range parts {
		kind, name, ok := strings.Cut(p, ".")
		if !ok {
			continue
		}
		out = append(out, Ref{Kind: kind, Name: name})
	}
	return out
}
