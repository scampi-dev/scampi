package engine

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"cuelang.org/go/cue/errors"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target"
	"golang.org/x/sync/errgroup"
)

const actionTimeout = 5 * time.Second

type opNode struct {
	op         spec.Op
	deps       []*opNode
	dependents []*opNode

	indegree int // original dependency count
	pending  int // runtime counter

	satisfied bool
}

func Apply(ctx context.Context, cfgPath string) error {
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		errs := errors.Errors(err)
		fmt.Printf("CUE error summary:\n%v\n", err)
		fmt.Printf("CUE error details:\n%v\n", errors.Details(err, nil))
		fmt.Printf("CUE: %d error(s)\n", len(errs))
		return err
	}

	fmt.Printf("decoded config: %#v\n", cfg)
	p, err := plan(cfg)
	if err != nil {
		return err
	}

	return executePlan(ctx, p)
}

func plan(cfg spec.Config) (spec.Plan, error) {
	p := spec.Plan{}

	for i, unit := range cfg.Units {
		act, err := unit.Type.Plan(i, unit.Config)
		if err != nil {
			return spec.Plan{}, err
		}
		p.Actions = append(p.Actions, act)
	}

	return p, nil
}

func executePlan(ctx context.Context, plan spec.Plan) error {
	for _, act := range plan.Actions {
		if err := executeAction(ctx, act); err != nil {
			return err
		}
	}

	return nil
}

func executeAction(ctx context.Context, act spec.Action) error {
	fmt.Printf("Running action: %s\n", act.Name())

	actCtx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	res, err := runAction(actCtx, act)
	if err != nil {
		return fmt.Errorf("action %s failed: %w", act.Name(), err)
	}

	if res.Changed {
		fmt.Printf("Action %s changed state\n", act.Name())
	} else {
		fmt.Printf("Action %s already in desired state\n", act.Name())
	}

	return nil
}

type scheduler struct {
	mu      sync.Mutex
	results []spec.Result
	grp     *errgroup.Group
	ctx     context.Context
}

func (s *scheduler) schedule(n *opNode, tgt target.Target) {
	if n.satisfied {
		return
	}

	s.grp.Go(func() error {
		fmt.Printf("Start op %s\n", n.op.Name())

		res, err := n.op.Execute(s.ctx, tgt)
		if err != nil {
			return err
		}

		fmt.Printf("Finished op %s\n", n.op.Name())

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
					s.schedule(d, tgt)
				}
			}
			s.mu.Unlock()
		} // critical section end

		return nil
	})
}

func (s *scheduler) runChecks(nodes []*opNode, tgt target.Target) error {
	g, ctx := errgroup.WithContext(s.ctx)

	for _, n := range nodes {
		n := n
		g.Go(func() error {
			res, err := n.op.Check(ctx, tgt)
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

func runAction(ctx context.Context, act spec.Action) (spec.Result, error) {
	nodes, err := buildPlan(act.Ops())
	tgt := target.LocalPosixTarget{}

	if err != nil {
		return spec.Result{}, err
	}

	s := &scheduler{}
	s.grp, s.ctx = errgroup.WithContext(ctx)

	if err := s.runChecks(nodes, tgt); err != nil {
		return spec.Result{}, err
	}

	s.initPending(nodes)

	for _, n := range nodes {
		if !n.satisfied && n.pending == 0 {
			s.schedule(n, tgt)
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
