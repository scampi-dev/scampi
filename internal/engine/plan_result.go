// SPDX-License-Identifier: GPL-3.0-only

package engine

import "scampi.dev/scampi/internal/diagnostic/result"

// Plan DTOs now live in result/; these are temporary alias shims for the #430
// migration, removed once engine call sites move to result/ directly. The
// builders below stay here - they operate on engine's *deployNode.
type (
	PlanResult  = result.Plan
	DeployLevel = result.DeployLevel
	DeployPlan  = result.DeployPlan
)

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
