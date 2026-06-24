// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

const defaultOpTimeout = 30 * time.Second

const opOutcomeUnknown = result.OpOutcome(0xff)

func opTimeout(op spec.Op) time.Duration {
	if t, ok := op.(spec.OpTimeout); ok {
		return t.Timeout()
	}
	return defaultOpTimeout
}

func validateOpReport(r result.OpReport) {
	switch r.Outcome {
	case result.OpSucceeded:
		if r.Result == nil || r.Err != nil {
			panic(errs.BUG("succeeded op must have result only, had error: %w", r.Err))
		}
	case result.OpFailed, result.OpAborted:
		if r.Err == nil || r.Result != nil {
			panic(errs.BUG("failed/aborted op must have err only, had result: %+v", r.Result))
		}
	case result.OpSkipped, result.OpWouldChange:
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

	outcome result.OpOutcome
	result  *spec.Result
	err     error
}

type scheduler struct {
	src source.Source
	tgt target.Target

	// action context
	actIdx    int
	actKind   string
	actDesc   string
	hookID    string // non-empty when running ops for a hook
	checkOnly bool   // true for check command (affects op event chattiness)

	// promised holds resources that upstream actions have promised to create.
	// Used during check mode to defer abort errors for resources that don't
	// exist yet.
	promised map[spec.Resource]bool

	mu      sync.Mutex
	results []spec.Result
	grp     *errgroup.Group
	ctx     diagnostic.Ctx
}

// stepRef and cause tag every event this scheduler emits with the action it
// belongs to and what triggered it (a hook, or nothing).
func (s *scheduler) stepRef() event.StepRef {
	return event.StepRef{Index: s.actIdx, Kind: s.actKind, Desc: s.actDesc}
}

func (s *scheduler) cause() event.Cause {
	if s.hookID != "" {
		return event.Cause{Kind: event.CauseHook, Ref: s.hookID}
	}
	return event.Cause{}
}

// emitChanges fires Change events for each drift item discovered
// by Check. Phase indicates whether this is a would-change report
// (check-only) or a did-change report (after apply). One Change per
// drift entry preserves field-level granularity in the consumer.
func (s *scheduler) emitChanges(displayID string, phase event.ChangePhase, drift []spec.DriftDetail) {
	if len(drift) == 0 {
		return
	}
	step := s.stepRef()
	cause := s.cause()
	for _, d := range drift {
		s.ctx.Emit(event.Change{
			Time:      time.Now(),
			Phase:     phase,
			Step:      step,
			DisplayID: displayID,
			Drift:     d,
			Cause:     cause,
		})
	}
}

// emitExecuted fires a Change event for an op that actually mutated state
// during Apply. Drift content is empty here - the op signalled "I changed
// something" without a per-field diff; the consumer falls back to a signal-
// only line when Drift.Field == "".
func (s *scheduler) emitExecuted(displayID string) {
	s.ctx.Emit(event.Change{
		Time:      time.Now(),
		Phase:     event.ChangeExecuted,
		Step:      s.stepRef(),
		DisplayID: displayID,
		Cause:     s.cause(),
	})
}

// emitResult fires the step-completion event once an action has settled,
// carrying its verdict and op breakdown. The verdict honors check vs apply:
// in check mode a "would change" counts as Changed.
func (s *scheduler) emitResult(sum result.ActionSummary) {
	outcome := event.StepUnchanged
	switch {
	case sum.Failed+sum.Aborted > 0:
		outcome = event.StepFailed
	case s.checkOnly && sum.WouldChange > 0:
		outcome = event.StepChanged
	case !s.checkOnly && sum.Changed > 0:
		outcome = event.StepChanged
	}
	s.ctx.Emit(event.Result{
		Time:    time.Now(),
		Step:    s.stepRef(),
		Outcome: outcome,
		Summary: event.StepSummary(sum),
		Cause:   s.cause(),
	})
}

func (s *scheduler) schedule(n *opNode) {
	if n.satisfied {
		return
	}

	s.grp.Go(func() error {
		displayID := diagnostic.OpDisplayID(n.op)

		opCtx, opCancel := context.WithTimeout(s.ctx, opTimeout(n.op))
		defer opCancel()

		res, err := n.op.Execute(opCtx, s.src, s.tgt)

		if err == nil && res.Changed {
			s.emitExecuted(displayID)
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		if err != nil {
			n.outcome = result.OpFailed
			n.err = err
			n.result = nil
			return err
		}

		n.outcome = result.OpSucceeded
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

			opCtx, opCancel := context.WithTimeout(ctx, opTimeout(n.op))
			defer opCancel()

			res, drift, err := n.op.Check(opCtx, s.src, s.tgt)
			if err != nil {
				if s.isDeferred(err) {
					s.mu.Lock()
					n.outcome = result.OpWouldChange
					s.mu.Unlock()
					return nil
				}

				impact, consumed := emitOpDiagnostic(s.ctx, s.actIdx, s.actKind, s.actDesc, displayID, err)

				if impact.ShouldAbort() {
					s.mu.Lock()
					n.outcome = result.OpAborted
					n.err = err
					s.mu.Unlock()
					return AbortError{Causes: []error{err}}
				}

				if !consumed {
					// Raw error (not a diagnostic) - propagate.
					return err
				}

				// Non-fatal diagnostic (warning) - proceed with the check result.
			}

			if !s.checkOnly {
				drift = nil
			}

			if s.checkOnly && res == spec.CheckUnsatisfied {
				s.emitChanges(displayID, event.ChangePlanned, drift)
			}

			s.mu.Lock()
			if res == spec.CheckSatisfied {
				n.satisfied = true
				n.outcome = result.OpSkipped
			} else {
				n.satisfied = false
			}
			s.mu.Unlock()
			return nil
		})
	}

	return g.Wait()
}

// isDeferred returns true when err references a missing resource that an
// upstream action has already promised to create.
func (s *scheduler) isDeferred(err error) bool {
	if len(s.promised) == 0 {
		return false
	}
	var d diagnostic.Deferrable
	if !errors.As(err, &d) {
		return false
	}
	res := d.DeferredResource()
	if s.promised[res] {
		return true
	}
	// A promised path like /foo/bar implies /foo will also exist
	// (MkdirAll semantics), so check if any promised path is a descendant.
	if res.Kind == spec.ResourcePath {
		for pp := range s.promised {
			if pp.Kind == spec.ResourcePath && strings.HasPrefix(pp.Name, res.Name+"/") {
				return true
			}
		}
	}
	return false
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

func (e *Engine) ExecutePlan(ctx diagnostic.Ctx, plan spec.Plan) (result.Execution, error) {
	res, err := e.executePlan(ctx, plan)
	if err != nil {
		return res, panicIfNotAbortError(err)
	}
	return res, nil
}

func (e *Engine) CheckPlan(ctx diagnostic.Ctx, plan spec.Plan) (result.Execution, map[spec.Resource]bool, error) {
	res, pp, err := e.checkPlan(ctx, plan)
	if err != nil {
		return res, pp, panicIfNotAbortError(err)
	}
	return res, pp, nil
}

func (e *Engine) checkPlan(ctx diagnostic.Ctx, plan spec.Plan) (result.Execution, map[spec.Resource]bool, error) {
	nodes := buildActionGraph(plan.Deploy.Actions)
	initActionPending(nodes)

	rep := result.Execution{
		Actions: make([]result.ActionReport, len(nodes)),
	}

	outputs := newStepOutputs()

	var mu sync.Mutex
	promised := map[spec.Resource]bool{}
	grp, gctx := errgroup.WithContext(ctx)

	var scheduleNode func(n *actionNode)
	scheduleNode = func(n *actionNode) {
		// Snapshot promised resources for this action under the lock
		snap := maps.Clone(promised)

		grp.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}

			// Resolve ref() markers before check (output available
			// from already-checked upstream steps).
			if r, ok := n.action.(refResolvable); ok {
				if err := r.ResolveRefs(buildRefResolver(outputs, true)); err != nil {
					emitActionDiagnostic(ctx, n.idx, n.action.Kind(), n.action.Desc(), err)
					abortErr := AbortError{Causes: []error{err}}
					mu.Lock()
					defer mu.Unlock()
					rep.Err = abortErr
					return abortErr
				}
			}

			res, err := e.checkAction(ctx.With(gctx), n.idx, n.action, snap)

			mu.Lock()
			defer mu.Unlock()

			rep.Actions[n.idx] = res

			// Capture step output for downstream refs.
			captureStepOutput(n.action, res, outputs)

			if err != nil {
				rep.Err = err
				return err
			}

			// If this action would change something, add its promised
			// resources to the set for downstream actions.
			if res.Summary.WouldChange > 0 {
				if p, ok := n.action.(spec.Promiser); ok {
					for _, key := range p.Promises() {
						promised[key] = true
					}
				}
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

	return rep, promised, err
}

func (e *Engine) checkAction(
	ctx diagnostic.Ctx,
	idx int,
	act spec.Action,
	promised map[spec.Resource]bool,
) (result.ActionReport, error) {
	return e.runCheckAction(ctx, idx, act, promised, "")
}

func (e *Engine) runCheckAction(
	ctx diagnostic.Ctx,
	idx int,
	act spec.Action,
	promised map[spec.Resource]bool,
	hookID string,
) (result.ActionReport, error) {
	nodes, planErr := buildPlan(act.Ops())
	if planErr != nil {
		return result.ActionReport{}, planErr
	}

	s := &scheduler{
		src:       e.src,
		tgt:       e.tgt,
		actIdx:    idx,
		actKind:   act.Kind(),
		actDesc:   act.Desc(),
		hookID:    hookID,
		checkOnly: true,
		promised:  promised,
	}
	grp, gctx := errgroup.WithContext(ctx)
	s.grp = grp
	s.ctx = ctx.With(gctx)

	checkErr := s.runChecks(nodes)

	// Unlike executeAction, we do NOT run the execution phase
	// Mark unsatisfied ops as OpWouldChange
	for _, n := range nodes {
		if n.outcome == opOutcomeUnknown {
			if checkErr != nil {
				n.outcome = result.OpAborted
				n.err = checkErr
			} else if !n.satisfied {
				n.outcome = result.OpWouldChange
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
		impact, consumed := emitActionDiagnostic(ctx, idx, act.Kind(), act.Desc(), checkErr)
		switch {
		case impact.ShouldAbort():
			err = AbortError{Causes: []error{checkErr}}
		case consumed:
			// Diagnostic emitted but didn't request a hard abort; ops
			// nonetheless landed in OpAborted state. Surface a non-nil
			// error so downstream actions don't run against a broken
			// upstream (#330).
			err = ActionAbortedError{Cause: checkErr}
		default:
			// Raw error that never went through the diagnostic pipeline —
			// pass through so panicIfNotAbortError raises a BUG panic.
			err = checkErr
		}
	}

	rep := buildActionReport(act, nodes)
	s.emitResult(rep.Summary)
	return rep, err
}

func (e *Engine) executePlan(ctx diagnostic.Ctx, plan spec.Plan) (result.Execution, error) {
	nodes := buildActionGraph(plan.Deploy.Actions)
	initActionPending(nodes)

	rep := result.Execution{
		Actions: make([]result.ActionReport, len(nodes)),
	}

	outputs := newStepOutputs()

	var mu sync.Mutex
	grp, gctx := errgroup.WithContext(ctx)

	var scheduleNode func(n *actionNode)
	scheduleNode = func(n *actionNode) {
		grp.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}

			// Resolve ref() markers before execution.
			if r, ok := n.action.(refResolvable); ok {
				if err := r.ResolveRefs(buildRefResolver(outputs, false)); err != nil {
					emitActionDiagnostic(ctx, n.idx, n.action.Kind(), n.action.Desc(), err)
					abortErr := AbortError{Causes: []error{err}}
					mu.Lock()
					defer mu.Unlock()
					rep.Err = abortErr
					return abortErr
				}
			}

			res, err := e.executeAction(ctx.With(gctx), n.idx, n.action)

			mu.Lock()
			defer mu.Unlock()

			rep.Actions[n.idx] = res

			// Capture step output for downstream refs.
			captureStepOutput(n.action, res, outputs)

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

// captureStepOutput stores a step's settled state in the output registry
// if the action has a step ID and any of its ops implement OutputProvider.
func captureStepOutput(act spec.Action, report result.ActionReport, outputs *stepOutputs) {
	id, ok := act.(stepIdentifier)
	if !ok {
		return
	}
	for _, opReport := range report.Ops {
		if provider, ok := opReport.Op.(spec.OutputProvider); ok {
			if out := provider.Output(); out != nil {
				outputs.Store(id.StepID(), out)
				return
			}
		}
	}
}

func (e *Engine) executeAction(ctx diagnostic.Ctx, idx int, act spec.Action) (result.ActionReport, error) {
	res, err := e.runAction(ctx, idx, act, "")
	if err != nil {
		return res, err
	}
	return res, nil
}

func (e *Engine) runAction(ctx diagnostic.Ctx, idx int, act spec.Action, hookID string) (result.ActionReport, error) {
	nodes, planErr := buildPlan(act.Ops())
	if planErr != nil {
		return result.ActionReport{}, planErr
	}

	s := &scheduler{
		src:     e.src,
		tgt:     e.tgt,
		actIdx:  idx,
		actKind: act.Kind(),
		actDesc: act.Desc(),
		hookID:  hookID,
	}
	grp, gctx := errgroup.WithContext(ctx)
	s.grp = grp
	s.ctx = ctx.With(gctx)

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
	if err != nil {
		for _, n := range nodes {
			if n.outcome == opOutcomeUnknown {
				n.outcome = result.OpAborted
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
		impact, consumed := emitActionDiagnostic(ctx, idx, act.Kind(), act.Desc(), err)
		switch {
		case impact.ShouldAbort():
			err = AbortError{Causes: []error{err}}
		case consumed:
			// Diagnostic emitted but didn't request a hard abort; ops
			// nonetheless landed in OpAborted state. Surface a non-nil
			// error so downstream actions don't run against a broken
			// upstream (#330).
			err = ActionAbortedError{Cause: err}
		default:
			// Raw error that never went through the diagnostic pipeline —
			// pass through so panicIfNotAbortError raises a BUG panic.
		}
	}

	rep := buildActionReport(act, nodes)
	s.emitResult(rep.Summary)
	return rep, err
}

func buildActionReport(act spec.Action, nodes []*opNode) result.ActionReport {
	var rep result.ActionReport
	rep.Action = act

	for _, n := range nodes {
		or := result.OpReport{
			Op:      n.op,
			Outcome: n.outcome,
			Result:  n.result,
			Err:     n.err,
		}
		validateOpReport(or)
		rep.Ops = append(rep.Ops, or)

		rep.Summary.Total++

		switch n.outcome {
		case result.OpSucceeded:
			rep.Summary.Succeeded++
			if n.result != nil && n.result.Changed {
				rep.Summary.Changed++
			}
		case result.OpFailed:
			rep.Summary.Failed++
		case result.OpAborted:
			rep.Summary.Aborted++
		case result.OpSkipped:
			rep.Summary.Skipped++
		case result.OpWouldChange:
			rep.Summary.WouldChange++
		}
	}

	return rep
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
