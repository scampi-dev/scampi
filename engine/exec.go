package engine

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/model"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"golang.org/x/sync/errgroup"
)

const actionTimeout = 5 * time.Second

const opOutcomeUnknown = model.OpOutcome(0xff)

func validateOpReport(r model.OpReport) {
	switch r.Outcome {
	case model.OpSucceeded:
		if r.Result == nil || r.Err != nil {
			panic(errs.BUG("succeeded op must have result only, had error: %w", r.Err))
		}
	case model.OpFailed, model.OpAborted:
		if r.Err == nil || r.Result != nil {
			panic(errs.BUG("failed/aborted op must have err only, had result: %+v", r.Result))
		}
	case model.OpSkipped, model.OpWouldChange:
		if r.Result != nil || r.Err != nil {
			panic(errs.BUG("skipped/would-change op must have no result or err"))
		}
	default:
		panic(errs.BUG("unknown op outcome"))
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

	// action context
	actIdx    int
	actKind   string
	actDesc   string
	checkOnly bool // true for check command (affects op event chattiness)

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
		displayID := diagnostic.OpDisplayID(n.op)

		s.em.EmitOpLifecycle(diagnostic.OpExecuteStarted(
			s.actIdx, s.actKind, s.actDesc, displayID,
		))

		res, err := n.op.Execute(s.ctx, s.src, s.tgt)

		s.em.EmitOpLifecycle(diagnostic.OpExecuted(
			s.actIdx, s.actKind, s.actDesc, displayID,
			res.Changed, time.Since(start), err,
		))

		s.mu.Lock()
		defer s.mu.Unlock()

		if err != nil {
			n.outcome = model.OpFailed
			n.err = err
			n.result = nil
			return err
		}

		n.outcome = model.OpSucceeded
		n.err = nil
		n.result = &res
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

		return nil
	})
}

func (s *scheduler) runChecks(nodes []*opNode) error {
	g, ctx := errgroup.WithContext(s.ctx)

	for _, n := range nodes {
		n := n
		g.Go(func() error {
			displayID := diagnostic.OpDisplayID(n.op)
			s.em.EmitOpLifecycle(diagnostic.OpCheckStarted(
				s.actIdx, s.actKind, s.actDesc, displayID,
			))

			res, err := n.op.Check(ctx, s.src, s.tgt)
			if err != nil {
				impact, consumed := emitOpDiagnostic(s.em, s.actIdx, s.actKind, s.actDesc, displayID, err)

				s.em.EmitOpLifecycle(diagnostic.OpChecked(
					s.actIdx, s.actKind, s.actDesc, displayID,
					res, err, s.checkOnly,
				))
				if impact.ShouldAbort() {
					s.mu.Lock()
					n.outcome = model.OpAborted
					n.err = err
					s.mu.Unlock()
					return AbortError{Causes: []error{err}}
				}

				if consumed {
					return nil
				}

				return err
			}

			s.em.EmitOpLifecycle(diagnostic.OpChecked(
				s.actIdx, s.actKind, s.actDesc, displayID,
				res, nil, s.checkOnly,
			))

			s.mu.Lock()
			if res == spec.CheckSatisfied {
				n.satisfied = true
				n.outcome = model.OpSkipped
			} else {
				n.satisfied = false
			}
			s.mu.Unlock()
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

func (e *Engine) CheckPlan(ctx context.Context, plan spec.Plan) (model.ExecutionReport, error) {
	res, err := e.checkPlan(ctx, plan)
	if err != nil {
		return res, panicIfNotAbortError(err)
	}
	return res, nil
}

func (e *Engine) checkPlan(ctx context.Context, plan spec.Plan) (model.ExecutionReport, error) {
	nodes := buildActionGraph(plan.Unit.Actions)
	initActionPending(nodes)

	rep := model.ExecutionReport{
		Actions: make([]model.ActionReport, len(nodes)),
	}

	var mu sync.Mutex
	grp, gctx := errgroup.WithContext(ctx)

	var scheduleNode func(n *actionNode)
	scheduleNode = func(n *actionNode) {
		grp.Go(func() error {
			// Check if context was cancelled before starting
			if err := gctx.Err(); err != nil {
				return err
			}

			res, err := e.checkAction(gctx, n.idx, n.action)

			mu.Lock()
			defer mu.Unlock()

			rep.Actions[n.idx] = res
			if err != nil {
				rep.Err = err
				return err
			}

			// Unblock actions that were waiting for this one
			for _, waiter := range n.requiredBy {
				waiter.pending--
				if waiter.pending == 0 {
					scheduleNode(waiter)
				}
			}

			return nil
		})
	}

	// Schedule actions with no dependencies
	mu.Lock()
	for _, n := range nodes {
		if n.pending == 0 {
			scheduleNode(n)
		}
	}
	mu.Unlock()

	err := grp.Wait()
	if err != nil {
		rep.Err = err
	}

	return rep, err
}

func (e *Engine) checkAction(ctx context.Context, idx int, act spec.Action) (model.ActionReport, error) {
	start := time.Now()
	kind := act.Kind()
	desc := act.Desc()
	e.em.EmitActionLifecycle(diagnostic.ActionStarted(idx, kind, desc))

	actCtx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	res, err := e.runCheckAction(actCtx, idx, act)

	e.em.EmitActionLifecycle(
		diagnostic.ActionFinished(
			idx,
			kind,
			desc,
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

func (e *Engine) runCheckAction(ctx context.Context, idx int, act spec.Action) (model.ActionReport, error) {
	nodes, planErr := buildPlan(act.Ops())
	if planErr != nil {
		return model.ActionReport{}, planErr
	}

	s := &scheduler{
		src:       e.src,
		tgt:       e.tgt,
		em:        e.em,
		actIdx:    idx,
		actKind:   act.Kind(),
		actDesc:   act.Desc(),
		checkOnly: true,
	}
	s.grp, s.ctx = errgroup.WithContext(ctx)

	checkErr := s.runChecks(nodes)

	// Unlike executeAction, we do NOT run the execution phase
	// Mark unsatisfied ops as OpWouldChange
	for _, n := range nodes {
		if n.outcome == opOutcomeUnknown {
			if checkErr != nil {
				n.outcome = model.OpAborted
				n.err = checkErr
			} else if !n.satisfied {
				n.outcome = model.OpWouldChange
			}
		}
	}

	// Enforce invariant: every op MUST have an outcome
	for _, n := range nodes {
		if n.outcome == opOutcomeUnknown {
			panic(errs.BUG("op left without outcome"))
		}
	}

	var err error
	if checkErr != nil {
		impact, consumed := emitActionDiagnostic(e.em, idx, act.Kind(), act.Desc(), checkErr)
		if impact.ShouldAbort() {
			err = AbortError{Causes: []error{checkErr}}
		} else if !consumed {
			err = checkErr
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
		case model.OpWouldChange:
			rep.Summary.WouldChange++
		}
	}

	return rep, err
}

func (e *Engine) executePlan(ctx context.Context, plan spec.Plan) (model.ExecutionReport, error) {
	nodes := buildActionGraph(plan.Unit.Actions)
	initActionPending(nodes)

	rep := model.ExecutionReport{
		Actions: make([]model.ActionReport, len(nodes)),
	}

	var mu sync.Mutex
	grp, gctx := errgroup.WithContext(ctx)

	var scheduleNode func(n *actionNode)
	scheduleNode = func(n *actionNode) {
		grp.Go(func() error {
			// Check if context was cancelled before starting
			if err := gctx.Err(); err != nil {
				return err
			}

			res, err := e.executeAction(gctx, n.idx, n.action)

			mu.Lock()
			defer mu.Unlock()

			rep.Actions[n.idx] = res
			if err != nil {
				rep.Err = err
				return err
			}

			// Unblock actions that were waiting for this one
			for _, waiter := range n.requiredBy {
				waiter.pending--
				if waiter.pending == 0 {
					scheduleNode(waiter)
				}
			}

			return nil
		})
	}

	// Schedule actions with no dependencies
	mu.Lock()
	for _, n := range nodes {
		if n.pending == 0 {
			scheduleNode(n)
		}
	}
	mu.Unlock()

	err := grp.Wait()
	if err != nil {
		rep.Err = err
	}

	return rep, err
}

func (e *Engine) executeAction(ctx context.Context, idx int, act spec.Action) (model.ActionReport, error) {
	start := time.Now()
	kind := act.Kind()
	desc := act.Desc()
	e.em.EmitActionLifecycle(diagnostic.ActionStarted(idx, kind, desc))

	actCtx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	res, err := e.runAction(actCtx, idx, act)

	e.em.EmitActionLifecycle(
		diagnostic.ActionFinished(
			idx,
			kind,
			desc,
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
	nodes, planErr := buildPlan(act.Ops())
	if planErr != nil {
		return model.ActionReport{}, planErr
	}

	s := &scheduler{
		src:     e.src,
		tgt:     e.tgt,
		em:      e.em,
		actIdx:  idx,
		actKind: act.Kind(),
		actDesc: act.Desc(),
	}
	s.grp, s.ctx = errgroup.WithContext(ctx)

	checkErr := s.runChecks(nodes)

	var execErr error
	if checkErr == nil {
		s.initPending(nodes)

		// Hold lock while reading node state to avoid race with goroutines
		// decrementing pending counts
		s.mu.Lock()
		for _, n := range nodes {
			if !n.satisfied && n.pending == 0 {
				s.schedule(n)
			}
		}
		s.mu.Unlock()

		execErr = s.grp.Wait()
	}

	// First error wins
	err := cmp.Or(checkErr, execErr)
	// Mark any ops without outcome as aborted
	if err != nil {
		for _, n := range nodes {
			if n.outcome == opOutcomeUnknown {
				n.outcome = model.OpAborted
				n.err = err
			}
		}
	}
	// Enforce invariant: every op MUST have an outcome
	for _, n := range nodes {
		if n.outcome == opOutcomeUnknown {
			panic(errs.BUG("op left without outcome"))
		}
	}

	if err != nil {
		impact, consumed := emitActionDiagnostic(e.em, idx, act.Kind(), act.Desc(), err)
		if impact.ShouldAbort() {
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
				panic(errs.BUG(
					"op %p depends on unknown op %p (StepType implementation error)",
					n.op, dep,
				))
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
		panic(errs.BUG("cycle detected in op graph (StepType implementation error)"))
	}

	for _, n := range nodes {
		n.pending = n.indegree
	}

	return slices.Collect(maps.Values(nodes)), nil
}
