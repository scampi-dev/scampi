package render

import "godoit.dev/doit/diagnostic/event"

// A DAG-ready view, renderer-only
type planDAG struct {
	Actions []dagAction
}

type dagAction struct {
	Index  int
	Name   string
	Kind   string
	Layers [][]event.PlannedOp // topologically layered ops
}

type renderNode struct {
	Op      event.PlannedOp
	Depth   int
	IsLast  bool   // last sibling at this depth
	Parents []bool // per depth: should a vertical bar continue?
}

func buildPlanDAG(detail event.PlanDetail) planDAG {
	var dag planDAG

	for _, act := range detail.Actions {
		layers := topoLayers(act.Ops)

		dag.Actions = append(dag.Actions, dagAction{
			Index:  act.Index,
			Name:   act.Name,
			Kind:   act.Kind,
			Layers: layers,
		})
	}

	return dag
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

func opDepth(op event.PlannedOp, index map[int]event.PlannedOp, memo map[int]int) int {
	if d, ok := memo[op.Index]; ok {
		return d
	}
	maxDepth := 0
	for _, dep := range op.DependsOn {
		if depOp, ok := index[dep]; ok {
			if v := opDepth(depOp, index, memo) + 1; v > maxDepth {
				maxDepth = v
			}
		}
	}
	memo[op.Index] = maxDepth
	return maxDepth
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
