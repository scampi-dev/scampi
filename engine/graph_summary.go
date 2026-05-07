// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"time"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
)

// emitGraphSummary surfaces the cross-deploy resource graph topology
// to the renderer once per scampi run. Suppressed for trivial graphs
// (single deploy or all-roots-with-no-deps) where there's nothing
// structural to show. See #276.
func emitGraphSummary(em diagnostic.Emitter, graph *deployGraph) {
	if em == nil || graph == nil || isTrivial(graph) {
		return
	}
	detail := event.GraphDetail{Levels: make([]event.GraphLevel, len(graph.levels))}
	for i, level := range graph.levels {
		nodes := make([]event.GraphNode, len(level))
		for j, n := range level {
			nodes[j] = event.GraphNode{
				DeployName: n.res.DeployName,
				TargetName: n.res.TargetName,
				After:      depNames(n),
				Needs:      driverResources(n),
			}
		}
		detail.Levels[i] = event.GraphLevel{Index: i, Nodes: nodes}
	}
	em.EmitGraph(event.GraphEvent{
		Time:       time.Now(),
		Detail:     detail,
		Severity:   signal.Info,
		Chattiness: event.Reserved,
	})
}

// isTrivial reports whether the graph has no structure worth showing
// — one level with no edges. Multi-level graphs always render; a
// single level with explicit deps (which can't happen but kept here
// for symmetry) would also render.
func isTrivial(graph *deployGraph) bool {
	if len(graph.levels) > 1 {
		return false
	}
	for _, level := range graph.levels {
		for _, n := range level {
			if len(n.deps) > 0 {
				return false
			}
		}
	}
	return true
}

func depNames(n *deployNode) []string {
	if len(n.deps) == 0 {
		return nil
	}
	out := make([]string, len(n.deps))
	for i, d := range n.deps {
		out[i] = d.res.DeployName
	}
	return out
}

// driverResources returns the resource names that drove n's dep
// edges — the inputs n declared. External inputs (no producer) are
// included too because they're informative ("needs realm:foo, but
// no producer in this run") and the renderer can decide how to
// distinguish them.
func driverResources(n *deployNode) []string {
	if len(n.deps) == 0 || len(n.inputs) == 0 {
		return nil
	}
	out := make([]string, len(n.inputs))
	for i, in := range n.inputs {
		out[i] = resourceKindName(in.Kind) + ":" + in.Name
	}
	return out
}
