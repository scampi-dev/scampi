// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"strings"

	"scampi.dev/scampi/internal/spec"
)

// actionNode represents an action in the dependency graph.
type actionNode struct {
	action      spec.Action
	idx         int
	requires    []*actionNode            // actions that must complete before this one
	requiresSet map[*actionNode]struct{} // O(1) dedup for addEdge
	requiredBy  []*actionNode            // actions that wait for this one
	pending     int                      // runtime counter for scheduling
}

// hasResources returns true if the action declares any resource inputs or
// promises. Actions without resources act as barriers in the dependency graph.
func hasResources(act spec.Action) bool {
	p, ok := act.(spec.Promiser)
	if !ok {
		return false
	}
	return len(p.Inputs()) > 0 || len(p.Promises()) > 0
}

// buildActionGraph constructs a dependency graph from actions based on their
// declared resource inputs and promises. Independent actions (no resource
// overlap) run in parallel; dependent actions run in order.
func buildActionGraph(actions []spec.Action) []*actionNode {
	nodes := make([]*actionNode, len(actions))
	for i, act := range actions {
		nodes[i] = &actionNode{action: act, idx: i}
	}

	// Map promised resources to the action that produces them.
	producers := map[spec.Resource]*actionNode{}
	for _, n := range nodes {
		if p, ok := n.action.(spec.Promiser); ok {
			for _, r := range p.Promises() {
				producers[r] = n
			}
		}
	}

	// For each action that consumes a resource, depend on the producer.
	// For path resources, also add parent-directory edges (a promised path
	// /foo/bar implies /foo exists via MkdirAll semantics).
	for _, n := range nodes {
		p, ok := n.action.(spec.Promiser)
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
	// Actions without resources act as barriers (memory fences): nothing
	// may reorder across them. Instead of connecting every barrier to every
	// other node (O(n²) edges), we chain consecutive barriers and fan edges
	// in/out to neighboring resource-aware nodes. This produces identical
	// execution order with O(n) edges.
	var lastBarrier *actionNode
	var resourceActionsSinceBarrier []*actionNode

	for _, n := range nodes {
		if hasResources(n.action) {
			if lastBarrier != nil {
				addEdge(lastBarrier, n)
			}
			resourceActionsSinceBarrier = append(resourceActionsSinceBarrier, n)
		} else {
			// n is a barrier
			if lastBarrier != nil {
				addEdge(lastBarrier, n)
			}
			for _, p := range resourceActionsSinceBarrier {
				addEdge(p, n)
			}
			resourceActionsSinceBarrier = resourceActionsSinceBarrier[:0]
			lastBarrier = n
		}
	}

	return nodes
}

// addEdge adds a dependency edge from -> to, skipping duplicates.
func addEdge(from, to *actionNode) {
	if to.requiresSet == nil {
		to.requiresSet = map[*actionNode]struct{}{}
	}
	if _, dup := to.requiresSet[from]; dup {
		return
	}
	to.requiresSet[from] = struct{}{}
	to.requires = append(to.requires, from)
	from.requiredBy = append(from.requiredBy, to)
}

// initActionPending sets pending counts based on unmet requirements.
func initActionPending(nodes []*actionNode) {
	for _, n := range nodes {
		n.pending = len(n.requires)
	}
}
