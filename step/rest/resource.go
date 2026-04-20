// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

type (
	Resource       struct{}
	ResourceConfig struct {
		_ struct{} `summary:"Declarative REST resource management with query/found/missing"`

		Desc     string                `step:"Human-readable description" optional:"true"`
		Query    *RequestConfig        `step:"Query request to check resource existence"`
		Missing  *RequestConfig        `step:"Request to execute when resource is not found" optional:"true"`
		Found    *RequestConfig        `step:"Request to execute when resource is found" optional:"true"`
		Bindings map[string]*JQBinding `step:"Named jq bindings resolved from query result" optional:"true"`
		State    map[string]any        `step:"Desired resource state" optional:"true"`
	}

	resourceAction struct {
		desc string
		step spec.StepInstance
		cfg  *ResourceConfig
	}
)

func (Resource) Kind() string   { return "rest.resource" }
func (Resource) NewConfig() any { return &ResourceConfig{} }

func (c *ResourceConfig) RefMaps() []map[string]any {
	var maps []map[string]any
	if c.State != nil {
		maps = append(maps, c.State)
	}
	if c.Missing != nil && c.Missing.Body != nil {
		if jb, ok := c.Missing.Body.(JSONBody); ok {
			if m, ok := jb.Data.(map[string]any); ok {
				maps = append(maps, m)
			}
		}
	}
	if c.Found != nil && c.Found.Body != nil {
		if jb, ok := c.Found.Body.(JSONBody); ok {
			if m, ok := jb.Data.(map[string]any); ok {
				maps = append(maps, m)
			}
		}
	}
	return maps
}

func (c *ResourceConfig) DedupKey() string {
	if c.Query == nil || c.Query.Check == nil {
		return ""
	}
	jq, ok := c.Query.Check.(*JQCheck)
	if !ok {
		return ""
	}
	return c.Query.Method + ":" + c.Query.Path + ":" + jq.Expr
}

func (Resource) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*ResourceConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &ResourceConfig{}, step.Config)
	}

	// bare-error: plan-time validation, surfaced via scampi source span
	invalid := func(format string, args ...any) error { return errs.Errorf(format, args...) }

	if cfg.Query == nil {
		return nil, invalid("rest.resource: query is required")
	}
	if cfg.Query.Check == nil {
		return nil, invalid("rest.resource: query must have a check (use rest.jq)")
	}
	if _, ok := cfg.Query.Check.(*JQCheck); !ok {
		return nil, invalid(
			"rest.resource: query check must be rest.jq, got %s",
			cfg.Query.Check.Kind(),
		)
	}
	if cfg.Missing == nil && cfg.Found == nil {
		return nil, invalid("rest.resource: at least one of missing or found is required")
	}
	if len(cfg.Bindings) > 0 && cfg.Found == nil {
		return nil, invalid("rest.resource: bindings require a found request")
	}

	return &resourceAction{
		desc: cfg.Desc,
		step: step,
		cfg:  cfg,
	}, nil
}

func (a *resourceAction) Desc() string { return a.desc }
func (a *resourceAction) Kind() string { return "rest.resource" }

func (a *resourceAction) Ops() []spec.Op {
	op := &resourceOp{
		query:    a.cfg.Query,
		missing:  a.cfg.Missing,
		found:    a.cfg.Found,
		bindings: a.cfg.Bindings,
		state:    a.cfg.State,
	}
	op.SetAction(a)
	return []spec.Op{op}
}

// StepID returns this action's step ID for output registry keying.
func (a *resourceAction) StepID() spec.StepID { return a.step.ID }

// Promiser
// -----------------------------------------------------------------------------

func (a *resourceAction) Promises() []spec.Resource {
	return []spec.Resource{spec.RefResource(a.step.ID)}
}

func (a *resourceAction) Inputs() []spec.Resource {
	var refs []spec.Resource
	for _, m := range a.cfg.RefMaps() {
		refs = append(refs, collectRefs(m)...)
	}
	return refs
}

// ResolveRefs replaces spec.Ref markers in all ref-bearing maps with
// concrete values from previously executed steps.
func (a *resourceAction) ResolveRefs(resolve spec.RefResolver) error {
	for _, m := range a.cfg.RefMaps() {
		if err := resolveMapRefs(m, resolve); err != nil {
			return err
		}
	}
	return nil
}

func collectRefs(m map[string]any) []spec.Resource {
	var refs []spec.Resource
	for _, v := range m {
		switch val := v.(type) {
		case spec.Ref:
			refs = append(refs, spec.RefResource(val.TargetID))
		case map[string]any:
			refs = append(refs, collectRefs(val)...)
		case []any:
			for _, elem := range val {
				if ref, ok := elem.(spec.Ref); ok {
					refs = append(refs, spec.RefResource(ref.TargetID))
				}
			}
		}
	}
	return refs
}

func resolveMapRefs(m map[string]any, resolve spec.RefResolver) error {
	for k, v := range m {
		switch val := v.(type) {
		case spec.Ref:
			resolved, err := resolve(val)
			if err != nil {
				return err
			}
			m[k] = resolved
		case map[string]any:
			if err := resolveMapRefs(val, resolve); err != nil {
				return err
			}
		case []any:
			for i, elem := range val {
				if ref, ok := elem.(spec.Ref); ok {
					resolved, err := resolve(ref)
					if err != nil {
						return err
					}
					val[i] = resolved
				}
			}
		}
	}
	return nil
}
