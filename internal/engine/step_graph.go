// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"strings"

	"scampi.dev/scampi/internal/spec"
)

// stepNode represents a step in the dependency graph.
type stepNode struct {
	step        spec.Step
	idx         int
	requires    []*stepNode            // steps that must complete before this one
	requiresSet map[*stepNode]struct{} // O(1) dedup for addEdge
	requiredBy  []*stepNode            // steps that wait for this one
	pending     int                    // runtime counter for scheduling
}

// hasResources returns true if the step declares any required or provided
// resources. Steps without resources act as barriers in the dependency graph.
func hasResources(act spec.Step) bool {
	if r, ok := act.(spec.StepRequires); ok && len(r.Requires()) > 0 {
		return true
	}
	if p, ok := act.(spec.StepProvides); ok && len(p.Provides()) > 0 {
		return true
	}
	return false
}

// buildStepGraph constructs a dependency graph from steps based on their
// declared resource requirements and provisions. Independent steps (no
// resource overlap) run in parallel; dependent steps run in order.
func buildStepGraph(steps []spec.Step) []*stepNode {
	nodes := make([]*stepNode, len(steps))
	for i, act := range steps {
		nodes[i] = &stepNode{step: act, idx: i}
	}

	// Map provided resources to the step that provides them.
	producers := map[spec.Resource]*stepNode{}
	for _, n := range nodes {
		if p, ok := n.step.(spec.StepProvides); ok {
			for _, r := range p.Provides() {
				producers[r] = n
			}
		}
	}

	// For each step that requires a resource, depend on the provider.
	// For path resources, also add parent-directory edges (a provided path
	// /foo/bar implies /foo exists via MkdirAll semantics).
	for _, n := range nodes {
		if r, ok := n.step.(spec.StepRequires); ok {
			for _, in := range r.Requires() {
				if producer := producers[in]; producer != nil && producer != n {
					addEdge(producer, n)
				}
			}
		}
		if p, ok := n.step.(spec.StepProvides); ok {
			for _, out := range p.Provides() {
				if out.Kind == spec.ResourcePath {
					for r, producer := range producers {
						if producer != n && r.Kind == spec.ResourcePath &&
							strings.HasPrefix(out.Name, r.Name+"/") {
							addEdge(producer, n)
						}
					}
				}
			}
		}
	}

	// Fence-based barrier edges
	// -----------------------------------------------------------------------------
	// Steps without resources act as barriers (memory fences): nothing
	// may reorder across them. Instead of connecting every barrier to every
	// other node (O(n^2) edges), we chain consecutive barriers and fan edges
	// in/out to neighboring resource-aware nodes. This produces identical
	// execution order with O(n) edges.
	var lastBarrier *stepNode
	var resourceStepsSinceBarrier []*stepNode

	for _, n := range nodes {
		if hasResources(n.step) {
			if lastBarrier != nil {
				addEdge(lastBarrier, n)
			}
			resourceStepsSinceBarrier = append(resourceStepsSinceBarrier, n)
		} else {
			// n is a barrier
			if lastBarrier != nil {
				addEdge(lastBarrier, n)
			}
			for _, p := range resourceStepsSinceBarrier {
				addEdge(p, n)
			}
			resourceStepsSinceBarrier = resourceStepsSinceBarrier[:0]
			lastBarrier = n
		}
	}

	return nodes
}

// addEdge adds a dependency edge from -> to, skipping duplicates.
func addEdge(from, to *stepNode) {
	if to.requiresSet == nil {
		to.requiresSet = map[*stepNode]struct{}{}
	}
	if _, dup := to.requiresSet[from]; dup {
		return
	}
	to.requiresSet[from] = struct{}{}
	to.requires = append(to.requires, from)
	from.requiredBy = append(from.requiredBy, to)
}

// initStepPending sets pending counts based on unmet requirements.
func initStepPending(nodes []*stepNode) {
	for _, n := range nodes {
		n.pending = len(n.requires)
	}
}
