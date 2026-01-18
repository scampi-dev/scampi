package engine

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"golang.org/x/sync/errgroup"
)

const actionTimeout = 5 * time.Second

type ExecResult struct {
	Res spec.Result
	Err error
}
type opNode struct {
	op         spec.Op
	deps       []*opNode
	dependents []*opNode

	indegree int // original dependency count
	pending  int // runtime counter

	satisfied bool
}
type scheduler struct {
	src source.Source
	tgt target.Target
	em  diagnostic.Emitter

	mu      sync.Mutex
	results []spec.Result
	grp     *errgroup.Group
	ctx     context.Context
}

func (s *scheduler) schedule(n *opNode) {
	if n.satisfied {
		return
	}

	s.grp.Go(func() error {
		start := time.Now()
		actionName := n.op.Action().Name()
		opName := n.op.Name()

		s.em.Emit(diagnostic.OpExecuteStarted(actionName, opName))

		res, err := n.op.Execute(s.ctx, s.src, s.tgt)

		s.em.Emit(diagnostic.OpExecuted(actionName, opName, res.Changed, time.Since(start), err))
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
					s.schedule(d)
				}
			}
			s.mu.Unlock()
		} // critical section end

		return nil
	})
}

func (s *scheduler) runChecks(nodes []*opNode) error {
	g, ctx := errgroup.WithContext(s.ctx)

	for _, n := range nodes {
		n := n
		g.Go(func() error {
			actionName := n.op.Action().Name()
			opName := n.op.Name()
			s.em.Emit(diagnostic.OpCheckStarted(actionName, opName))

			res, err := n.op.Check(ctx, s.src, s.tgt)
			if err != nil {
				dr, consumed := emitDiagnostics(
					s.em,
					event.Subject{
						Action: actionName,
						Op:     opName,
						Kind:   n.op.Action().Kind(),
						Name:   n.op.Action().Name(),
					},
					err,
				)

				s.em.Emit(diagnostic.OpChecked(actionName, opName, res, err))
				if dr.ShouldAbort() {
					return AbortError{Causes: []error{err}}
				}

				if consumed {
					return nil
				}

				return err
			}

			s.em.Emit(diagnostic.OpChecked(actionName, opName, res, nil))
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

func (e *Engine) ExecutePlan(ctx context.Context, plan spec.Plan) ([]ExecResult, error) {
	res, err := e.executePlan(ctx, plan)
	if err != nil {
		return res, panicIfNotAbortError(err)
	}
	return res, nil
}

func (e *Engine) executePlan(ctx context.Context, plan spec.Plan) ([]ExecResult, error) {
	var results []ExecResult

	for _, act := range plan.Actions {
		res, err := e.executeAction(ctx, act)
		if err != nil {
			return []ExecResult{}, err
		}

		results = append(results, ExecResult{res, err})
	}

	return results, nil
}

func (e *Engine) executeAction(ctx context.Context, act spec.Action) (spec.Result, error) {
	start := time.Now()
	name := act.Name()
	e.em.Emit(diagnostic.ActionStarted(name))

	actCtx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	res, err := e.runAction(actCtx, act)
	e.em.Emit(diagnostic.ActionFinished(name, res.Changed, time.Since(start), err))
	if err != nil {
		return spec.Result{}, fmt.Errorf("action %s failed: %w", name, err)
	}

	return res, nil
}

func (e *Engine) runAction(ctx context.Context, act spec.Action) (spec.Result, error) {
	nodes, err := buildPlan(act.Ops())
	if err != nil {
		return spec.Result{}, err
	}

	s := &scheduler{
		src: e.src,
		tgt: e.tgt,
		em:  e.em,
	}
	s.grp, s.ctx = errgroup.WithContext(ctx)

	if err := s.runChecks(nodes); err != nil {
		return spec.Result{}, err
	}

	s.initPending(nodes)

	for _, n := range nodes {
		if !n.satisfied && n.pending == 0 {
			s.schedule(n)
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
