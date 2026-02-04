package engine

import "godoit.dev/doit/spec"

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

	// For each action that reads a path, depend on the writer
	for _, n := range nodes {
		if p, ok := n.action.(spec.Pather); ok {
			for _, in := range p.InputPaths() {
				if writer := writers[in]; writer != nil && writer != n {
					n.requires = append(n.requires, writer)
					writer.requiredBy = append(writer.requiredBy, n)
				}
			}
		}
	}

	// Actions without Pather run sequentially (each depends on previous)
	// to preserve fail-fast for unknown side effects
	var prevNonPather *actionNode
	for _, n := range nodes {
		if _, ok := n.action.(spec.Pather); !ok {
			if prevNonPather != nil {
				n.requires = append(n.requires, prevNonPather)
				prevNonPather.requiredBy = append(prevNonPather.requiredBy, n)
			}
			prevNonPather = n
		}
	}

	return nodes
}

// initActionPending sets pending counts based on unmet requirements.
func initActionPending(nodes []*actionNode) {
	for _, n := range nodes {
		n.pending = len(n.requires)
	}
}
