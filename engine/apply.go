package engine

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"cuelang.org/go/cue/errors"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"golang.org/x/sync/errgroup"
)

const actionTimeout = 5 * time.Second

type (
	execResult struct {
		res spec.Result
		err error
	}
	opNode struct {
		op         spec.Op
		deps       []*opNode
		dependents []*opNode

		indegree int // original dependency count
		pending  int // runtime counter

		satisfied bool
	}
)

func Apply(ctx context.Context, em diagnostic.Emitter, cfgPath string) error {
	start := time.Now()
	em.Emit(diagnostic.EngineStarted())

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		errs := errors.Errors(err)
		// FIXME: diagnostic
		fmt.Printf("CUE error summary:\n%v\n", err)
		fmt.Printf("CUE error details:\n%v\n", errors.Details(err, nil))
		fmt.Printf("CUE: %d error(s)\n", len(errs))
		return err
	}

	plan, err := plan(cfg, em)
	if err != nil {
		return err
	}

	// em.EngineFinish(changed bool, duration time.Duration)
	results, err := executePlan(ctx, em, plan)
	if err != nil {
		// FIXME: diagnostic
		return err
	}

	rs := diagnostic.RunSummary{
		ChangedCount: 0,
		FailedCount:  0,
		TotalCount:   len(results),
	}
	for _, res := range results {
		if res.res.Changed {
			rs.ChangedCount++
		}
		if res.err != nil {
			rs.FailedCount++
		}
	}

	em.Emit(diagnostic.EngineFinished(rs, time.Since(start), err))

	return err
}

func plan(cfg spec.Config, em diagnostic.Emitter) (spec.Plan, error) {
	start := time.Now()
	em.Emit(diagnostic.PlanStarted())

	var (
		plan     spec.Plan
		problems []event.PlanProblem
		causes   []error
	)

	for i, unit := range cfg.Units {
		act, err := unit.Type.Plan(i, unit)
		if err != nil {
			emitDiagnostics(em, err, event.Subject{
				Index: i,
				Name:  unit.Name,
				Kind:  unit.Type.Kind(),
			})

			causes = append(causes, err)
			problems = append(problems, event.PlanProblem{
				Index: i,
				Name:  unit.Name,
				Kind:  unit.Type.Kind(),
				Err:   err,
			})
			continue
		}

		plan.Actions = append(plan.Actions, act)
		em.Emit(diagnostic.UnitPlanned(i, act.Name(), unit.Type.Kind()))
	}

	em.Emit(diagnostic.PlanFinished(
		len(plan.Actions),
		time.Since(start),
		problems,
	))

	if len(problems) > 0 {
		return spec.Plan{}, AbortError{
			Causes: causes,
		}
	}

	return plan, nil
}

func executePlan(ctx context.Context, em diagnostic.Emitter, plan spec.Plan) ([]execResult, error) {
	var results []execResult

	for _, act := range plan.Actions {
		res, err := executeAction(ctx, em, act)
		if err != nil {
			return []execResult{}, err
		}

		results = append(results, execResult{res, err})
	}

	return results, nil
}

func executeAction(ctx context.Context, em diagnostic.Emitter, act spec.Action) (spec.Result, error) {
	start := time.Now()
	name := act.Name()
	em.Emit(diagnostic.ActionStarted(name))

	actCtx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	res, err := runAction(actCtx, em, act)
	em.Emit(diagnostic.ActionFinished(name, res.Changed, time.Since(start), err))
	if err != nil {
		return spec.Result{}, fmt.Errorf("action %s failed: %w", name, err)
	}

	return res, nil
}

type scheduler struct {
	mu      sync.Mutex
	results []spec.Result
	grp     *errgroup.Group
	ctx     context.Context
}

func (s *scheduler) schedule(n *opNode, em diagnostic.Emitter, tgt target.Target) {
	if n.satisfied {
		return
	}

	s.grp.Go(func() error {
		start := time.Now()
		actionName := n.op.Action()
		opName := n.op.Name()

		em.Emit(diagnostic.OpExecuteStarted(actionName, opName))

		res, err := n.op.Execute(s.ctx, tgt)

		em.Emit(diagnostic.OpExecuted(actionName, opName, res.Changed, time.Since(start), err))
		if err != nil {
			return err
		}

		{ // critical section start
			s.mu.Lock()
			s.results = append(s.results, res)

			// unblock unsatisfied dependents
			for _, d := range n.dependents {
				if d.satisfied {
					continue
				}

				d.pending--
				if d.pending == 0 {
					s.schedule(d, em, tgt)
				}
			}
			s.mu.Unlock()
		} // critical section end

		return nil
	})
}

func (s *scheduler) runChecks(nodes []*opNode, em diagnostic.Emitter, tgt target.Target) error {
	g, ctx := errgroup.WithContext(s.ctx)

	for _, n := range nodes {
		n := n
		g.Go(func() error {
			actionName := n.op.Action()
			opName := n.op.Name()
			em.Emit(diagnostic.OpCheckStarted(actionName, opName))

			res, err := n.op.Check(ctx, tgt)
			em.Emit(diagnostic.OpChecked(actionName, opName, res, err))
			if err != nil {
				return err
			}

			n.satisfied = (res == spec.CheckSatisfied)
			return nil
		})
	}

	return g.Wait()
}

func (s *scheduler) initPending(nodes []*opNode) {
	for _, n := range nodes {
		n.pending = 0
	}

	for _, n := range nodes {
		if n.satisfied {
			continue
		}

		for _, d := range n.dependents {
			if !d.satisfied {
				d.pending++
			}
		}
	}
}

func runAction(ctx context.Context, em diagnostic.Emitter, act spec.Action) (spec.Result, error) {
	nodes, err := buildPlan(act.Ops())
	tgt := target.LocalPosixTarget{}

	if err != nil {
		return spec.Result{}, err
	}

	s := &scheduler{}
	s.grp, s.ctx = errgroup.WithContext(ctx)

	if err := s.runChecks(nodes, em, tgt); err != nil {
		return spec.Result{}, err
	}

	s.initPending(nodes)

	for _, n := range nodes {
		if !n.satisfied && n.pending == 0 {
			s.schedule(n, em, tgt)
		}
	}

	if err := s.grp.Wait(); err != nil {
		return spec.Result{}, err
	}

	changed := false
	for _, res := range s.results {
		if res.Changed {
			changed = true
			break
		}
	}

	return spec.Result{Changed: changed}, nil
}

func buildPlan(ops []spec.Op) ([]*opNode, error) {
	nodes := map[spec.Op]*opNode{}

	for _, op := range ops {
		nodes[op] = &opNode{op: op}
	}

	for _, n := range nodes {
		for _, dep := range n.op.DependsOn() {
			dn, ok := nodes[dep]
			if !ok {
				return nil, fmt.Errorf(
					"op %q depends on unknown op %q",
					n.op.Name(), dep.Name(),
				)
			}

			n.deps = append(n.deps, dn)
			dn.dependents = append(dn.dependents, n)
			n.indegree++
		}
	}

	tmp := make(map[*opNode]int)
	for _, n := range nodes {
		tmp[n] = n.indegree
	}

	var queue []*opNode
	for n, deg := range tmp {
		if deg == 0 {
			queue = append(queue, n)
		}
	}

	visited := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visited++

		for _, d := range n.dependents {
			tmp[d]--
			if tmp[d] == 0 {
				queue = append(queue, d)
			}
		}
	}

	if visited != len(nodes) {
		return nil, fmt.Errorf("cycle detected in op graph")
	}

	for _, n := range nodes {
		n.pending = n.indegree
	}

	return slices.Collect(maps.Values(nodes)), nil
}

func emitDiagnostics(em diagnostic.Emitter, err error, subject event.Subject) {
	var dp diagnostic.DiagnosticProvider
	if errors.As(err, &dp) {
		for _, ev := range dp.Diagnostics(subject) {
			em.Emit(ev)
		}
	}
}
