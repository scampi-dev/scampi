// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"strings"

	"scampi.dev/scampi/spec"
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

// buildActionGraph constructs a dependency graph from actions based on their
// declared input/output paths. Independent actions (no path overlap) run in
// parallel; dependent actions run in order.
func buildActionGraph(actions []spec.Action) []*actionNode {
	nodes := make([]*actionNode, len(actions))
	for i, act := range actions {
		nodes[i] = &actionNode{action: act, idx: i}
	}

	// Map output paths to the action that writes them
	writers := map[string]*actionNode{}
	for _, n := range nodes {
		if p, ok := n.action.(spec.Pather); ok {
			for _, out := range p.OutputPaths() {
				writers[out] = n
			}
		}
	}

	// For each action that reads a path, depend on the writer.
	// For each action that writes to a path, depend on the action that
	// creates its parent directory (path-prefix matching).
	for _, n := range nodes {
		p, ok := n.action.(spec.Pather)
		if !ok {
			continue
		}
		for _, in := range p.InputPaths() {
			if writer := writers[in]; writer != nil && writer != n {
				addEdge(writer, n)
			}
		}
		for _, out := range p.OutputPaths() {
			for dir, writer := range writers {
				if writer != n && strings.HasPrefix(out, dir+"/") {
					addEdge(writer, n)
				}
			}
		}
	}

	// Fence-based barrier edges
	// -----------------------------------------------------------------------------
	// Non-Pather actions act as barriers (memory fences): nothing may reorder
	// across them. Instead of connecting every barrier to every other node
	// (O(n²) edges), we chain consecutive barriers and fan edges in/out to
	// neighboring Pather nodes. This produces identical execution order with
	// O(n) edges.
	var lastBarrier *actionNode
	var pathersSinceBarrier []*actionNode

	for _, n := range nodes {
		if _, ok := n.action.(spec.Pather); ok {
			if lastBarrier != nil {
				addEdge(lastBarrier, n)
			}
			pathersSinceBarrier = append(pathersSinceBarrier, n)
		} else {
			// n is a barrier
			if lastBarrier != nil {
				addEdge(lastBarrier, n)
			}
			for _, p := range pathersSinceBarrier {
				addEdge(p, n)
			}
			pathersSinceBarrier = pathersSinceBarrier[:0]
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
