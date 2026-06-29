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

// hasResources returns true if the step declares any resource inputs or
// promises. Steps without resources act as barriers in the dependency graph.
func hasResources(act spec.Step) bool {
	p, ok := act.(spec.Promiser)
	if !ok {
		return false
	}
	return len(p.Inputs()) > 0 || len(p.Promises()) > 0
}

// buildStepGraph constructs a dependency graph from steps based on their
// declared resource inputs and promises. Independent steps (no resource
// overlap) run in parallel; dependent steps run in order.
func buildStepGraph(steps []spec.Step) []*stepNode {
	nodes := make([]*stepNode, len(steps))
	for i, act := range steps {
		nodes[i] = &stepNode{step: act, idx: i}
	}

	// Map promised resources to the step that produces them.
	producers := map[spec.Resource]*stepNode{}
	for _, n := range nodes {
		if p, ok := n.step.(spec.Promiser); ok {
			for _, r := range p.Promises() {
				producers[r] = n
			}
		}
	}

	// For each step that consumes a resource, depend on the producer.
	// For path resources, also add parent-directory edges (a promised path
	// /foo/bar implies /foo exists via MkdirAll semantics).
	for _, n := range nodes {
		p, ok := n.step.(spec.Promiser)
		if !ok {
			continue
		}
		for _, in := range p.Inputs() {
			if producer := producers[in]; producer != nil && producer != n {
				addEdge(producer, n)
			}
		}
		for _, out := range p.Promises() {
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

	// Fence-based barrier edges
	// -----------------------------------------------------------------------------
	// Steps without resources act as barriers (memory fences): nothing
	// may reorder across them. Instead of connecting every barrier to every
	// other node (O(n²) edges), we chain consecutive barriers and fan edges
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
