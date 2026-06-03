// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"maps"
	"slices"
	"strings"
)

// Inventory is the set of refs scampi currently manages, along with
// the attrs needed to destroy each one and the deps recorded at apply
// time (used to order destroys: reverse of apply order means a file
// inside a dir gets removed before its parent). Built up by Fold over
// an event stream (action log replay) or by direct Add/Remove during
// reconcile. Not safe for concurrent use.
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

// Get returns copies of the stored attrs and deps so callers can pass
// them onward (e.g. into Kind.Destroy) without holding a handle to the
// inventory's internal state.
func (i *Inventory) Get(ref Ref) (attrs Attrs, deps []Ref, ok bool) {
	e, ok := i.entries[ref]
	if !ok {
		return nil, nil, false
	}
	return maps.Clone(e.attrs), slices.Clone(e.deps), true
}

// Orphans returns the inventory entries whose refs are not in the
// declared set. Those are the resources the reconciler should destroy.
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

// Fold integrates one event into the inventory. Unknown codes are
// ignored so the projection survives future code additions. `deps`
// is pulled out of the attrs as a comma-separated `kind.name,...`
// string; missing or empty means no deps.
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
