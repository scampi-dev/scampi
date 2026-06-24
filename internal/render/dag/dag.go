// SPDX-License-Identifier: GPL-3.0-only

// Package dag provides DAG utilities for plan rendering.
package dag

import (
	"slices"

	"scampi.dev/scampi/internal/diagnostic/result"
)

// PlanDAG is a DAG-ready view of a plan, used by renderers.
type PlanDAG struct {
	ActionLayers [][]Action // actions grouped by parallel execution layer
}

// Action represents an action in the plan DAG.
type Action struct {
	Index     int
	Desc      string
	Kind      string
	DependsOn []int                // action indices this depends on
	Layers    [][]result.PlannedOp // topologically layered ops within action
}

// Build constructs a DAG view of the plan detail.
func Build(detail result.PlanDetail) PlanDAG {
	actions := make([]Action, len(detail.Actions))
	for i, act := range detail.Actions {
		actions[i] = Action{
			Index:     act.Index,
			Desc:      act.Desc,
			Kind:      act.Kind,
			DependsOn: act.DependsOn,
			Layers:    topoLayers(act.Ops),
		}
	}

	return PlanDAG{
		ActionLayers: topoLayersActions(actions),
	}
}

// topoLayersActions groups actions by their topological layer.
// Actions in the same layer have no dependencies on each other and can run in parallel.
func topoLayersActions(actions []Action) [][]Action {
	if len(actions) == 0 {
		return nil
	}

	inDegree := make(map[int]int)
	children := make(map[int][]int)
	index := make(map[int]Action)

	for _, act := range actions {
		index[act.Index] = act
		inDegree[act.Index] = len(act.DependsOn)
		for _, dep := range act.DependsOn {
			children[dep] = append(children[dep], act.Index)
		}
	}

	var layers [][]Action
	var ready []int

	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}
	slices.Sort(ready) // stable ordering

	for len(ready) > 0 {
		var next []int
		var layer []Action

		for _, id := range ready {
			layer = append(layer, index[id])
			for _, child := range children[id] {
				inDegree[child]--
				if inDegree[child] == 0 {
					next = append(next, child)
				}
			}
		}

		slices.SortFunc(layer, func(a, b Action) int {
			return a.Index - b.Index
		})

		layers = append(layers, layer)
		slices.Sort(next)
		ready = next
	}

	return layers
}

func topoLayers(ops []result.PlannedOp) [][]result.PlannedOp {
	inDegree := make(map[int]int)
	children := make(map[int][]int)
	index := make(map[int]result.PlannedOp)

	for _, op := range ops {
		index[op.Index] = op
		inDegree[op.Index] = len(op.DependsOn)
		for _, dep := range op.DependsOn {
			children[dep] = append(children[dep], op.Index)
		}
	}

	var layers [][]result.PlannedOp
	var ready []int

	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}
	slices.Sort(ready)

	for len(ready) > 0 {
		var next []int
		var layer []result.PlannedOp

		for _, id := range ready {
			layer = append(layer, index[id])
			for _, child := range children[id] {
				inDegree[child]--
				if inDegree[child] == 0 {
					next = append(next, child)
				}
			}
		}

		slices.SortFunc(layer, func(a, b result.PlannedOp) int {
			return a.Index - b.Index
		})

		layers = append(layers, layer)
		slices.Sort(next)
		ready = next
	}

	return layers
}

// FlattenLayers flattens layered ops into a single slice.
func FlattenLayers(layers [][]result.PlannedOp) []result.PlannedOp {
	var out []result.PlannedOp
	for _, l := range layers {
		out = append(out, l...)
	}
	return out
}

// BuildDepTree builds a map of op index to its children.
func BuildDepTree(ops []result.PlannedOp) map[int][]result.PlannedOp {
	children := make(map[int][]result.PlannedOp)

	for _, op := range ops {
		for _, dep := range op.DependsOn {
			children[dep] = append(children[dep], op)
		}
	}

	return children
}

// FindRoots returns ops that have no dependencies.
func FindRoots(ops []result.PlannedOp) []result.PlannedOp {
	hasParent := make(map[int]bool)
	for _, op := range ops {
		if len(op.DependsOn) > 0 {
			hasParent[op.Index] = true
		}
	}

	var roots []result.PlannedOp
	for _, op := range ops {
		if !hasParent[op.Index] {
			roots = append(roots, op)
		}
	}
	return roots
}
