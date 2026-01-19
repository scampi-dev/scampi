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
	"godoit.dev/doit/model"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"godoit.dev/doit/util"
	"golang.org/x/sync/errgroup"
)

const actionTimeout = 5 * time.Second

const opOutcomeUnknown = model.OpOutcome(0xff)

func validateOpReport(r model.OpReport) {
	switch r.Outcome {
	case model.OpSucceeded:
		if r.Result == nil || r.Err != nil {
			panic("BUG: succeeded op must have result only")
		}
	case model.OpFailed, model.OpAborted:
		if r.Err == nil || r.Result != nil {
			panic("BUG: failed/aborted op must have err only")
		}
	case model.OpSkipped:
		if r.Result != nil || r.Err != nil {
			panic("BUG: skipped op must have no result or err")
		}
	default:
		panic("BUG: unknown op outcome")
	}
}

type opNode struct {
	op         spec.Op
	deps       []*opNode
	dependents []*opNode

	indegree int // original dependency count
	pending  int // runtime counter

	satisfied bool

	outcome model.OpOutcome
	result  *spec.Result
	err     error
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
		if err == nil {
			n.outcome = model.OpSucceeded
			n.err = nil
			n.result = &res
		} else {
			n.outcome = model.OpFailed
			n.err = err
			n.result = nil
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
					},
					err,
				)

				s.em.Emit(diagnostic.OpChecked(actionName, opName, res, err))
				if dr.ShouldAbort() {
					n.outcome = model.OpAborted
					n.err = err
					return AbortError{Causes: []error{err}}
				}

				if consumed {
					return nil
				}

				return err
			}

			s.em.Emit(diagnostic.OpChecked(actionName, opName, res, nil))
			if res == spec.CheckSatisfied {
				n.satisfied = true
				n.outcome = model.OpSkipped
				return nil
			}

			n.satisfied = false
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

func (e *Engine) ExecutePlan(ctx context.Context, plan spec.Plan) (model.ExecutionReport, error) {
	res, err := e.executePlan(ctx, plan)
	if err != nil {
		return res, panicIfNotAbortError(err)
	}
	return res, nil
}

func (e *Engine) executePlan(ctx context.Context, plan spec.Plan) (model.ExecutionReport, error) {
	var rep model.ExecutionReport

	for i, act := range plan.Actions {
		res, err := e.executeAction(ctx, i, act)
		rep.Actions = append(rep.Actions, res)
		if err != nil {
			rep.Err = err
			return rep, err
		}

	}

	return rep, nil
}

func (e *Engine) executeAction(ctx context.Context, idx int, act spec.Action) (model.ActionReport, error) {
	start := time.Now()
	name := act.Name()
	e.em.Emit(diagnostic.ActionStarted(name))

	actCtx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	res, err := e.runAction(actCtx, idx, act)

	e.em.Emit(
		diagnostic.ActionFinished(
			name,
			res.Summary,
			time.Since(start),
			err,
		),
	)

	if err != nil {
		return res, err
	}

	return res, nil
}

func (e *Engine) runAction(ctx context.Context, idx int, act spec.Action) (model.ActionReport, error) {
	nodes, err := buildPlan(act.Ops())
	if err != nil {
		return model.ActionReport{}, err
	}

	s := &scheduler{
		src: e.src,
		tgt: e.tgt,
		em:  e.em,
	}
	s.grp, s.ctx = errgroup.WithContext(ctx)

	err = s.runChecks(nodes)
	if err != nil {
		// mark remaining ops as aborted
		for _, n := range nodes {
			if n.outcome == opOutcomeUnknown {
				n.outcome = model.OpAborted
				n.err = err
			}
		}
		// SKIP scheduling entirely
		goto buildReport
	}

	s.initPending(nodes)

	for _, n := range nodes {
		if !n.satisfied && n.pending == 0 {
			s.schedule(n)
		}
	}

	err = s.grp.Wait()
	if err != nil {
		for _, n := range nodes {
			if n.outcome == opOutcomeUnknown {
				n.outcome = model.OpAborted
				n.err = err
			}
		}
	}

buildReport:
	// Enforce invariant: every op MUST have an outcome
	for _, n := range nodes {
		if n.outcome == opOutcomeUnknown {
			panic(util.BUG("op left without outcome"))
		}
	}

	if err != nil {
		dr, consumed := emitDiagnostics(
			e.em,
			event.Subject{
				Index:  idx,
				Action: act.Name(),
				Kind:   act.Kind(),
			},
			err,
		)
		if dr.ShouldAbort() {
			err = AbortError{Causes: []error{err}}
		} else if consumed {
			err = nil
		}
	}

	// Build ActionReport
	var rep model.ActionReport
	rep.Action = act

	for _, n := range nodes {
		or := model.OpReport{
			Op:      n.op,
			Outcome: n.outcome,
			Result:  n.result,
			Err:     n.err,
		}
		validateOpReport(or)
		rep.Ops = append(rep.Ops, or)

		rep.Summary.Total++

		switch n.outcome {
		case model.OpSucceeded:
			rep.Summary.Succeeded++
			if n.result != nil && n.result.Changed {
				rep.Summary.Changed++
			}
		case model.OpFailed:
			rep.Summary.Failed++
		case model.OpAborted:
			rep.Summary.Aborted++
		case model.OpSkipped:
			rep.Summary.Skipped++
		}
	}

	return rep, err
}

func buildPlan(ops []spec.Op) ([]*opNode, error) {
	nodes := map[spec.Op]*opNode{}

	for _, op := range ops {
		nodes[op] = &opNode{
			op: op,
			// explicit invariants
			outcome: opOutcomeUnknown,
			result:  nil,
			err:     nil,
		}
	}

	for _, n := range nodes {
		for _, dep := range n.op.DependsOn() {
			dn, ok := nodes[dep]
			if !ok {
				// FIXME: error
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
		// FIXME: error
		return nil, fmt.Errorf("cycle detected in op graph")
	}

	for _, n := range nodes {
		n.pending = n.indegree
	}

	return slices.Collect(maps.Values(nodes)), nil
}
