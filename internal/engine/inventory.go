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

// Diff returns the apply set (the declared list, every entry) and the
// destroy set (entries in the inventory but not declared). Apply is
// always the full declared list because reconcile is idempotent: in-
// sync resources noop on apply.
func (i *Inventory) Diff(declared []Resource) (toApply, toDestroy []Ref) {
	declaredSet := make(map[Ref]bool, len(declared))
	toApply = make([]Ref, 0, len(declared))
	for _, r := range declared {
		ref := r.Ref()
		declaredSet[ref] = true
		toApply = append(toApply, ref)
	}
	for ref := range i.entries {
		if !declaredSet[ref] {
			toDestroy = append(toDestroy, ref)
		}
	}
	return toApply, toDestroy
}

// Fold mutates the inventory in response to one action-log event. The
// caller (action-log replay) parses each line and dispatches here.
// Unknown codes are ignored so the projection survives future code
// additions.
func (i *Inventory) Fold(code Code, ref Ref, attrs map[string]string) {
	switch code {
	case CodeApplySuccess:
		i.Add(ref, attrs)
	case CodeDestroySuccess:
		i.Remove(ref)
	}
}
