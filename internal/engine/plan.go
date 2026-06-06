// SPDX-License-Identifier: GPL-3.0-only

package engine

import "context"

// Plan inspects what a Reconcile against dir would do without touching
// live state. Reads inv; never mutates it or writes lifecycle events.
type Plan struct {
	Create  []Ref // would write a new resource (state=missing)
	Update  []Ref // would rewrite (state=diverging: drift or take-over)
	Adopt   []Ref // would claim matching live state (first time + adopt)
	Halt    []Ref // would refuse: live exists but adopt=false
	Destroy []Ref // would remove orphans
	InSync  []Ref // owned and matching - no action
}

type PlanConfig struct {
	Dir       string
	Inventory *Inventory
	Emitter   Emitter
}

func MakePlan(ctx context.Context, cfg PlanConfig) (*Plan, error) {
	dir, inv := cfg.Dir, cfg.Inventory
	log := NewLog(cfg.Emitter)
	snap, err := snapshot(ctx, dir, log)
	if err != nil {
		return nil, err
	}
	p := &Plan{}
	for _, r := range snap {
		ref := r.Ref()
		was := inv.Has(ref)
		k, err := kindFor(r)
		if err != nil {
			continue
		}
		state, err := k.Check(ctx, r)
		if err != nil {
			return nil, err
		}
		switch {
		case was && state == StateMatching:
			p.InSync = append(p.InSync, ref)
		case !was && state != StateMissing && !r.Attrs.GetBool("adopt"):
			p.Halt = append(p.Halt, ref)
		case state == StateMissing:
			p.Create = append(p.Create, ref)
		case state == StateMatching:
			p.Adopt = append(p.Adopt, ref)
		default:
			p.Update = append(p.Update, ref)
		}
	}
	p.Destroy = inv.Orphans(snap)
	return p, nil
}
