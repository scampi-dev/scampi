// SPDX-License-Identifier: GPL-3.0-only

package engine

import "maps"

// Inventory is the set of refs scampi currently manages, with the
// attrs needed to later destroy each one. Built up by Fold over an
// event stream (action log replay) or by direct Add/Remove during
// reconcile. Not safe for concurrent use.
type Inventory struct {
	entries map[Ref]map[string]string
}

func NewInventory() *Inventory {
	return &Inventory{entries: map[Ref]map[string]string{}}
}

func (i *Inventory) Add(ref Ref, attrs map[string]string) {
	i.entries[ref] = maps.Clone(attrs)
}

func (i *Inventory) Remove(ref Ref) { delete(i.entries, ref) }

func (i *Inventory) Has(ref Ref) bool {
	_, ok := i.entries[ref]
	return ok
}

// Get returns a copy of the stored attrs so callers can pass them
// onward (e.g. into Kind.Destroy) without holding a handle to the
// inventory's internal state.
func (i *Inventory) Get(ref Ref) (map[string]string, bool) {
	a, ok := i.entries[ref]
	return maps.Clone(a), ok
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
// ignored so the projection survives future code additions.
func (i *Inventory) Fold(code Code, ref Ref, attrs map[string]string) {
	switch code {
	case CodeApplySuccess:
		i.Add(ref, attrs)
	case CodeDestroySuccess:
		i.Remove(ref)
	}
}
