// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"strings"

	"scampi.dev/scampi/spec"
)

// actionNode represents an action in the dependency graph.
type actionNode struct {
	action     spec.Action
	idx        int
	requires   []*actionNode // actions that must complete before this one
	requiredBy []*actionNode // actions that wait for this one
	pending    int           // runtime counter for scheduling
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

	// Non-Pather actions (e.g. run steps) act as barriers: they depend on
	// all preceding actions, and all subsequent actions depend on them.
	// The engine can't see what an opaque command reads or writes, so it
	// must not reorder anything across it — same idea as a memory fence.
	for i, n := range nodes {
		if _, ok := n.action.(spec.Pather); ok {
			continue
		}
		for _, prev := range nodes[:i] {
			addEdge(prev, n)
		}
		for _, next := range nodes[i+1:] {
			addEdge(n, next)
		}
	}

	return nodes
}

// addEdge adds a dependency edge from -> to, skipping duplicates.
func addEdge(from, to *actionNode) {
	for _, r := range to.requires {
		if r == from {
			return
		}
	}
	to.requires = append(to.requires, from)
	from.requiredBy = append(from.requiredBy, to)
}

// initActionPending sets pending counts based on unmet requirements.
func initActionPending(nodes []*actionNode) {
	for _, n := range nodes {
		n.pending = len(n.requires)
	}
}
