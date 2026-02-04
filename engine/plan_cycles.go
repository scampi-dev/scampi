package engine

import (
	"fmt"
	"strings"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type CyclicDependency struct {
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

func (e CyclicDependency) Error() string {
	ids := make([]string, 0, len(e.Cycle))
	for _, op := range e.Cycle {
		ids = append(ids, opID(op))
	}
	return "cyclic dependency: " + strings.Join(ids, " -> ")
}

func (e CyclicDependency) EventTemplate() event.Template {
	ids := make([]string, 0, len(e.Cycle))
	for _, op := range e.Cycle {
		ids = append(ids, opID(op))
	}

	return event.Template{
		ID:   "engine.CyclicDependency",
		Text: "cyclic dependency detected",
		Hint: strings.Join(ids, " -> "),
	}
}

func (e CyclicDependency) Severity() signal.Severity { return signal.Error }
func (CyclicDependency) Impact() diagnostic.Impact   { return diagnostic.ImpactAbort }

func DetectPlanCycles(em diagnostic.Emitter, plan spec.Plan) error {
	cycles := detectPlanCycles(plan)
	if len(cycles) > 0 {
		var err AbortError
		for _, cycle := range cycles {
			cd := CyclicDependency{Cycle: cycle}
			err.Causes = append(err.Causes, cd)

			em.EmitPlanDiagnostic(diagnostic.RaisePlanDiagnostic(
				0, // step index not applicable for cycle detection
				cycle[0].Action().Kind(),
				cycle[0].Action().Desc(),
				cd,
			))
		}

		return err
	}

	return nil
}

func detectPlanCycles(plan spec.Plan) [][]spec.Op {
	visited := map[spec.Op]bool{}
	onStack := map[spec.Op]bool{}

	var stack []spec.Op
	var cycles [][]spec.Op

	var dfs func(op spec.Op)
	dfs = func(op spec.Op) {
		if onStack[op] {
			// cycle found: extract suffix of stack
			cycle := extractCycle(stack, op)
			cycles = append(cycles, cycle)
			return
		}
		if visited[op] {
			return
		}

		visited[op] = true
		onStack[op] = true
		stack = append(stack, op)

		for _, dep := range op.DependsOn() {
			dfs(dep)
		}

		stack = stack[:len(stack)-1]
		onStack[op] = false
	}

	for _, a := range plan.Unit.Actions {
		for _, op := range a.Ops() {
			if !visited[op] {
				dfs(op)
			}
		}
	}

	return dedupeCycles(cycles)
}

func extractCycle(stack []spec.Op, start spec.Op) []spec.Op {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == start {
			cycle := append([]spec.Op{}, stack[i:]...)
			cycle = append(cycle, start) // close the loop explicitly
			return cycle
		}
	}
	panic("cycle start not found in stack (bug)")
}

func dedupeCycles(cycles [][]spec.Op) [][]spec.Op {
	seen := map[string]bool{}
	var out [][]spec.Op

	for _, c := range cycles {
		key := cycleKey(c)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func cycleKey(cycle []spec.Op) string {
	// ignore final repeated node for keying
	n := len(cycle) - 1

	// find rotation with minimal pointer string
	minIdx := 0
	for i := 1; i < n; i++ {
		if ptr(cycle[i]) < ptr(cycle[minIdx]) {
			minIdx = i
		}
	}

	var key strings.Builder
	for i := range n {
		key.WriteString(ptr(cycle[(minIdx+i)%n]) + ">")
	}
	return key.String()
}

func ptr(op spec.Op) string {
	return fmt.Sprintf("%p", op)
}

// ActionCyclicDependency represents a cycle in the action dependency graph.
type ActionCyclicDependency struct {
	Cycle []spec.Action
}

func actionID(act spec.Action) string {
	if act.Desc() != "" {
		return act.Desc()
	}
	return fmt.Sprintf("%s@%p", act.Kind(), act)
}

func (e ActionCyclicDependency) Error() string {
	ids := make([]string, 0, len(e.Cycle))
	for _, act := range e.Cycle {
		ids = append(ids, actionID(act))
	}
	return "cyclic action dependency: " + strings.Join(ids, " -> ")
}

func (e ActionCyclicDependency) EventTemplate() event.Template {
	ids := make([]string, 0, len(e.Cycle))
	for _, act := range e.Cycle {
		ids = append(ids, actionID(act))
	}

	return event.Template{
		ID:   "engine.ActionCyclicDependency",
		Text: "cyclic action dependency detected",
		Hint: strings.Join(ids, " -> "),
	}
}

func (e ActionCyclicDependency) Severity() signal.Severity { return signal.Error }
func (ActionCyclicDependency) Impact() diagnostic.Impact   { return diagnostic.ImpactAbort }

// DetectActionCycles checks for cycles in the action dependency graph.
func DetectActionCycles(em diagnostic.Emitter, nodes []*actionNode) error {
	cycles := detectActionCycles(nodes)
	if len(cycles) > 0 {
		var err AbortError
		for _, cycle := range cycles {
			cd := ActionCyclicDependency{Cycle: cycle}
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

func detectActionCycles(nodes []*actionNode) [][]spec.Action {
	visited := map[*actionNode]bool{}
	onStack := map[*actionNode]bool{}

	var stack []*actionNode
	var cycles [][]spec.Action

	var dfs func(n *actionNode)
	dfs = func(n *actionNode) {
		if onStack[n] {
			// cycle found: extract suffix of stack
			cycle := extractActionCycle(stack, n)
			cycles = append(cycles, cycle)
			return
		}
		if visited[n] {
			return
		}

		visited[n] = true
		onStack[n] = true
		stack = append(stack, n)

		for _, req := range n.requires {
			dfs(req)
		}

		stack = stack[:len(stack)-1]
		onStack[n] = false
	}

	for _, n := range nodes {
		if !visited[n] {
			dfs(n)
		}
	}

	return dedupeActionCycles(cycles)
}

func extractActionCycle(stack []*actionNode, start *actionNode) []spec.Action {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == start {
			cycle := make([]spec.Action, 0, len(stack)-i+1)
			for j := i; j < len(stack); j++ {
				cycle = append(cycle, stack[j].action)
			}
			cycle = append(cycle, start.action) // close the loop explicitly
			return cycle
		}
	}
	panic("cycle start not found in stack (bug)")
}

func dedupeActionCycles(cycles [][]spec.Action) [][]spec.Action {
	seen := map[string]bool{}
	var out [][]spec.Action

	for _, c := range cycles {
		key := actionCycleKey(c)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func actionCycleKey(cycle []spec.Action) string {
	// ignore final repeated node for keying
	n := len(cycle) - 1

	// find rotation with minimal pointer string
	minIdx := 0
	for i := 1; i < n; i++ {
		if fmt.Sprintf("%p", cycle[i]) < fmt.Sprintf("%p", cycle[minIdx]) {
			minIdx = i
		}
	}

	var key strings.Builder
	for i := range n {
		key.WriteString(fmt.Sprintf("%p", cycle[(minIdx+i)%n]) + ">")
	}
	return key.String()
}
