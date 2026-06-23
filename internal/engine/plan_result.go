// SPDX-License-Identifier: GPL-3.0-only

package engine

import "scampi.dev/scampi/internal/diagnostic/event"

// PlanResult is the return value of the top-level Plan entry point.
// It is the cross-deploy execution schedule, with each deploy's
// detailed action plan attached as a leaf node. There is no separate
// "graph" axis: Levels is always populated, and a single-deploy run
// is just one level containing one node with empty After/Needs.
type PlanResult struct {
	Levels []DeployLevel
}

// DeployLevel is one rank in the cross-deploy topology. Nodes within
// a level can plan and execute concurrently; every node in level N
// must finish before any node in level N+1 starts.
type DeployLevel struct {
	Index int
	Nodes []DeployPlan
}

// DeployPlan carries the planned actions for one deploy alongside the
// edges that placed it in its level. After and Needs are nil for
// roots; the renderer uses that to decide whether to draw a graph
// header section at all.
type DeployPlan struct {
	DeployName string
	TargetName string
	After      []string         // deploy names this one waits on
	Needs      []string         // resource names that drove After
	Detail     event.PlanDetail // the action DAG for this deploy
}

// isTrivial reports whether the graph has no structure worth a graph
// header - a single node with no incoming edges. The plan tree still
// renders; only the cross-deploy header section is suppressed.
func (r PlanResult) isTrivial() bool {
	if len(r.Levels) != 1 || len(r.Levels[0].Nodes) != 1 {
		return false
	}
	return len(r.Levels[0].Nodes[0].After) == 0
}

// HasGraph reports whether the result is worth rendering a cross-
// deploy graph header for. Renderers should call this before drawing
// the [graph] section.
func (r PlanResult) HasGraph() bool { return !r.isTrivial() }

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
// edges - the inputs n declared. External inputs (no producer) are
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
