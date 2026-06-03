// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// topoSort runs Kahn's algorithm on a ref set with a per-ref dep
// lookup. Edges pointing outside the input set are ignored, so the
// result for orphan subsets is still well-defined. Ties preserve
// input order so output is stable across runs.
func topoSort(refs []Ref, depsOf func(Ref) []Ref) ([]Ref, error) {
	refSet := make(map[Ref]bool, len(refs))
	for _, r := range refs {
		refSet[r] = true
	}
	indeg := make(map[Ref]int, len(refs))
	dependents := make(map[Ref][]Ref, len(refs))
	for _, r := range refs {
		indeg[r] = 0
	}
	for _, r := range refs {
		for _, dep := range depsOf(r) {
			if !refSet[dep] {
				continue
			}
			indeg[r]++
			dependents[dep] = append(dependents[dep], r)
		}
	}
	queue := make([]Ref, 0, len(refs))
	for _, r := range refs {
		if indeg[r] == 0 {
			queue = append(queue, r)
		}
	}
	out := make([]Ref, 0, len(refs))
	for len(queue) > 0 {
		r := queue[0]
		queue = queue[1:]
		out = append(out, r)
		for _, d := range dependents[r] {
			indeg[d]--
			if indeg[d] == 0 {
				queue = append(queue, d)
			}
		}
	}
	if len(out) != len(refs) {
		var cyclic []string
		for _, r := range refs {
			if indeg[r] > 0 {
				cyclic = append(cyclic, r.String())
			}
		}
		sort.Strings(cyclic)
		return nil, fmt.Errorf("dependency cycle: %s", strings.Join(cyclic, ", "))
	}
	return out, nil
}

func topoSortResources(resources []Resource) ([]Resource, error) {
	refs := make([]Ref, len(resources))
	byRef := make(map[Ref]Resource, len(resources))
	for i, r := range resources {
		refs[i] = r.Ref()
		byRef[r.Ref()] = r
	}
	sorted, err := topoSort(refs, func(r Ref) []Ref {
		return byRef[r].deps
	})
	if err != nil {
		return nil, err
	}
	out := make([]Resource, len(sorted))
	for i, r := range sorted {
		out[i] = byRef[r]
	}
	return out, nil
}

// destroyOrder sorts orphans so dependents come before their deps:
// reverse of apply order. A cycle here is defensive fallback;
// validate catches cycles in declared state.
func destroyOrder(orphans []Ref, inv *Inventory) []Ref {
	sorted, err := topoSort(orphans, func(r Ref) []Ref {
		_, deps, _ := inv.Get(r)
		return deps
	})
	if err != nil {
		return slices.Clone(orphans)
	}
	slices.Reverse(sorted)
	return sorted
}
