// SPDX-License-Identifier: GPL-3.0-only

// Package dag provides DAG utilities for plan rendering.
package dag

import (
	"slices"

	"scampi.dev/scampi/internal/diagnostic/result"
)

// PlanDAG is a DAG-ready view of a plan, used by renderers.
type PlanDAG struct {
	StepLayers [][]Step // steps grouped by parallel execution layer
}

// Step represents a step in the plan DAG.
type Step struct {
	Index     int
	Desc      string
	Kind      string
	DependsOn []int                // step indices this depends on
	Layers    [][]result.PlannedOp // topologically layered ops within step
}

// Build constructs a DAG view of the plan detail.
func Build(detail result.PlanDetail) PlanDAG {
	steps := make([]Step, len(detail.Steps))
	for i, act := range detail.Steps {
		steps[i] = Step{
			Index:     act.Index,
			Desc:      act.Desc,
			Kind:      act.Kind,
			DependsOn: act.DependsOn,
			Layers:    topoLayers(act.Ops),
		}
	}

	return PlanDAG{
		StepLayers: topoLayersSteps(steps),
	}
}

// topoLayersSteps groups steps by their topological layer.
// Steps in the same layer have no dependencies on each other and can run in parallel.
func topoLayersSteps(steps []Step) [][]Step {
	if len(steps) == 0 {
		return nil
	}

	inDegree := make(map[int]int)
	children := make(map[int][]int)
	index := make(map[int]Step)

	for _, act := range steps {
		index[act.Index] = act
		inDegree[act.Index] = len(act.DependsOn)
		for _, dep := range act.DependsOn {
			children[dep] = append(children[dep], act.Index)
		}
	}

	var layers [][]Step
	var ready []int

	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}
	slices.Sort(ready) // stable ordering

	for len(ready) > 0 {
		var next []int
		var layer []Step

		for _, id := range ready {
			layer = append(layer, index[id])
			for _, child := range children[id] {
				inDegree[child]--
				if inDegree[child] == 0 {
					next = append(next, child)
				}
			}
		}

		slices.SortFunc(layer, func(a, b Step) int {
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
