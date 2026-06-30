// SPDX-License-Identifier: GPL-3.0-only

package engine

// depNames and driverResources build the cross-deploy edge labels for a
// result.DeployPlan from engine's *deployNode. The DTOs themselves live in
// result/; these stay here because they reach into engine-internal graph state.

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
		out[i] = displayResource(in)
	}
	return out
}
