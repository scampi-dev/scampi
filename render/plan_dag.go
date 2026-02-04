package render

import (
	"slices"

	"godoit.dev/doit/diagnostic/event"
)

// A DAG-ready view, renderer-only
type planDAG struct {
	ActionLayers [][]dagAction // actions grouped by parallel execution layer
}

type dagAction struct {
	Index     int
	Desc      string
	Kind      string
	DependsOn []int               // action indices this depends on
	Layers    [][]event.PlannedOp // topologically layered ops within action
}

func buildPlanDAG(detail event.PlanDetail) planDAG {
	// Build dagAction list
	actions := make([]dagAction, len(detail.Actions))
	for i, act := range detail.Actions {
		actions[i] = dagAction{
			Index:     act.Index,
			Desc:      act.Desc,
			Kind:      act.Kind,
			DependsOn: act.DependsOn,
			Layers:    topoLayers(act.Ops),
		}
	}

	// Group actions into topological layers (parallel execution levels)
	actionLayers := topoLayersActions(actions)

	return planDAG{
		ActionLayers: actionLayers,
	}
}

// topoLayersActions groups actions by their topological layer.
// Actions in the same layer have no dependencies on each other and can run in parallel.
func topoLayersActions(actions []dagAction) [][]dagAction {
	if len(actions) == 0 {
		return nil
	}

	inDegree := make(map[int]int)
	children := make(map[int][]int)
	index := make(map[int]dagAction)

	for _, act := range actions {
		index[act.Index] = act
		inDegree[act.Index] = len(act.DependsOn)
		for _, dep := range act.DependsOn {
			children[dep] = append(children[dep], act.Index)
		}
	}

	var layers [][]dagAction
	var ready []int

	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}
	slices.Sort(ready) // stable ordering

	for len(ready) > 0 {
		var next []int
		var layer []dagAction

		for _, id := range ready {
			layer = append(layer, index[id])
			for _, child := range children[id] {
				inDegree[child]--
				if inDegree[child] == 0 {
					next = append(next, child)
				}
			}
		}

		// Sort layer by index for stable output
		slices.SortFunc(layer, func(a, b dagAction) int {
			return a.Index - b.Index
		})

		layers = append(layers, layer)
		slices.Sort(next) // stable ordering for next iteration
		ready = next
	}

	return layers
}

func topoLayers(ops []event.PlannedOp) [][]event.PlannedOp {
	inDegree := make(map[int]int)
	children := make(map[int][]int)
	index := make(map[int]event.PlannedOp)

	for _, op := range ops {
		index[op.Index] = op
		inDegree[op.Index] = len(op.DependsOn)
		for _, dep := range op.DependsOn {
			children[dep] = append(children[dep], op.Index)
		}
	}

	var layers [][]event.PlannedOp
	var ready []int

	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}

	for len(ready) > 0 {
		var next []int
		var layer []event.PlannedOp

		for _, id := range ready {
			layer = append(layer, index[id])
			for _, child := range children[id] {
				inDegree[child]--
				if inDegree[child] == 0 {
					next = append(next, child)
				}
			}
		}

		layers = append(layers, layer)
		ready = next
	}

	return layers
}

func flattenLayers(layers [][]event.PlannedOp) []event.PlannedOp {
	var out []event.PlannedOp
	for _, l := range layers {
		out = append(out, l...)
	}
	return out
}

func buildDepTree(ops []event.PlannedOp) map[int][]event.PlannedOp {
	children := make(map[int][]event.PlannedOp)

	for _, op := range ops {
		for _, dep := range op.DependsOn {
			children[dep] = append(children[dep], op)
		}
	}

	return children
}

func findRoots(ops []event.PlannedOp) []event.PlannedOp {
	hasParent := make(map[int]bool)
	for _, op := range ops {
		if len(op.DependsOn) > 0 {
			hasParent[op.Index] = true
		}
	}

	var roots []event.PlannedOp
	for _, op := range ops {
		if !hasParent[op.Index] {
			roots = append(roots, op)
		}
	}
	return roots
}
