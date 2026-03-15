// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"fmt"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

type CyclicDependencyError struct {
	Cycle []spec.Op
}

// opID returns an identifier for an op (template ID if available, otherwise pointer)
func opID(op spec.Op) string {
	if d, ok := op.(spec.OpDescriber); ok {
		if desc := d.OpDescription(); desc != nil {
			return desc.PlanTemplate().ID
		}
	}
	return fmt.Sprintf("%p", op)
}

func (e CyclicDependencyError) Error() string {
	ids := make([]string, 0, len(e.Cycle))
	for _, op := range e.Cycle {
		ids = append(ids, opID(op))
	}
	return "cyclic dependency: " + strings.Join(ids, " -> ")
}

func (e CyclicDependencyError) EventTemplate() event.Template {
	ids := make([]string, 0, len(e.Cycle))
	for _, op := range e.Cycle {
		ids = append(ids, opID(op))
	}

	return event.Template{
		ID:   "engine.CyclicDependency",
		Text: "cyclic dependency detected",
		Hint: `{{join " -> " .}}`,
		Data: ids,
	}
}

func (e CyclicDependencyError) Severity() signal.Severity { return signal.Error }
func (CyclicDependencyError) Impact() diagnostic.Impact   { return diagnostic.ImpactAbort }

func DetectPlanCycles(em diagnostic.Emitter, plan spec.Plan) error {
	var roots []spec.Op
	for _, a := range plan.Unit.Actions {
		roots = append(roots, a.Ops()...)
	}

	cycles := dedupCycles(
		detectCycles(roots, func(op spec.Op) []spec.Op { return op.DependsOn() }),
		ptrKey[spec.Op],
	)

	if len(cycles) > 0 {
		var err AbortError
		for _, cycle := range cycles {
			cd := CyclicDependencyError{Cycle: cycle}
			err.Causes = append(err.Causes, cd)

			em.EmitPlanDiagnostic(diagnostic.RaisePlanDiagnostic(
				0,
				cycle[0].Action().Kind(),
				cycle[0].Action().Desc(),
				cd,
			))
		}

		return err
	}

	return nil
}

// ActionCyclicDependencyError represents a cycle in the action dependency graph.
type ActionCyclicDependencyError struct {
	Cycle []spec.Action
}

func actionID(act spec.Action) string {
	if act.Desc() != "" {
		return act.Desc()
	}
	return fmt.Sprintf("%s@%p", act.Kind(), act)
}

func (e ActionCyclicDependencyError) Error() string {
	ids := make([]string, 0, len(e.Cycle))
	for _, act := range e.Cycle {
		ids = append(ids, actionID(act))
	}
	return "cyclic action dependency: " + strings.Join(ids, " -> ")
}

func (e ActionCyclicDependencyError) EventTemplate() event.Template {
	ids := make([]string, 0, len(e.Cycle))
	for _, act := range e.Cycle {
		ids = append(ids, actionID(act))
	}

	return event.Template{
		ID:   "engine.ActionCyclicDependency",
		Text: "cyclic action dependency detected",
		Hint: `{{join " -> " .}}`,
		Data: ids,
	}
}

func (e ActionCyclicDependencyError) Severity() signal.Severity { return signal.Error }
func (ActionCyclicDependencyError) Impact() diagnostic.Impact   { return diagnostic.ImpactAbort }

// detectHookCycles finds cycles in hook on_change chains using DFS.
func detectHookCycles(em diagnostic.Emitter, hooks map[string][]spec.StepInstance) error {
	if len(hooks) == 0 {
		return nil
	}

	// Build adjacency: hook ID → hook IDs it references via on_change
	adj := map[string][]string{}
	roots := make([]string, 0, len(hooks))
	for id, steps := range hooks {
		roots = append(roots, id)
		for _, step := range steps {
			adj[id] = append(adj[id], step.OnChange...)
		}
	}

	cycles := detectCycles(roots, func(id string) []string { return adj[id] })
	if len(cycles) > 0 {
		cycle := cycles[0]
		source := findCycleEdgeSource(hooks, cycle)
		err := HookCycleError{Chain: cycle, Source: source}
		em.EmitPlanDiagnostic(diagnostic.RaisePlanDiagnostic(
			-1,
			"hook",
			cycle[0],
			err,
		))
		return AbortError{Causes: []error{err}}
	}

	return nil
}

// findCycleEdgeSource locates the on_change field span for the edge that
// closes the cycle. The cycle slice is [A, ..., X, A] so the closing edge
// is from X → A.
func findCycleEdgeSource(hooks map[string][]spec.StepInstance, cycle []string) spec.SourceSpan {
	from := cycle[len(cycle)-2]
	to := cycle[len(cycle)-1]
	for _, step := range hooks[from] {
		for _, ref := range step.OnChange {
			if ref == to {
				if fs, ok := step.Fields["on_change"]; ok {
					return fs.Value
				}
				return step.Source
			}
		}
	}
	return spec.SourceSpan{}
}

// DetectActionCycles checks for cycles in the action dependency graph.
func DetectActionCycles(em diagnostic.Emitter, nodes []*actionNode) error {
	rawCycles := dedupCycles(
		detectCycles(nodes, func(n *actionNode) []*actionNode { return n.requires }),
		ptrKey[*actionNode],
	)

	if len(rawCycles) > 0 {
		var err AbortError
		for _, raw := range rawCycles {
			cycle := make([]spec.Action, len(raw))
			for i, n := range raw {
				cycle[i] = n.action
			}

			cd := ActionCyclicDependencyError{Cycle: cycle}
			err.Causes = append(err.Causes, cd)

			em.EmitPlanDiagnostic(diagnostic.RaisePlanDiagnostic(
				0,
				cycle[0].Kind(),
				cycle[0].Desc(),
				cd,
			))
		}

		return err
	}

	return nil
}
