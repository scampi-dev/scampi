// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

type (
	ResourceSet       struct{}
	ResourceSetConfig struct {
		_ struct{} `summary:"Declarative REST set reconciliation with key-based matching"`

		Desc         string                `step:"Human-readable description" optional:"true"`
		Query        *RequestConfig        `step:"Query to fetch the full remote set"`
		Key          CheckConfig           `step:"jq expression to extract match key from each item"`
		Items        []any                 `step:"Desired set of items" optional:"true"`
		Missing      *RequestConfig        `step:"Request for items in declared set but not remote" optional:"true"`
		Found        *RequestConfig        `step:"Request for items in both sets with drift" optional:"true"`
		Orphan       *RequestConfig        `step:"Request for items in remote but not declared" optional:"true"`
		OrphanFilter CheckConfig           `step:"jq filter for orphan narrowing" optional:"true"`
		Bindings     map[string]*JQBinding `step:"Per-item bindings from matched remote object" optional:"true"`
		OrphanState  map[string]any        `step:"State to send for orphan items" optional:"true"`
		Promises     []string              `step:"Cross-deploy resources this step produces" optional:"true"`
		Inputs       []string              `step:"Cross-deploy resources this step consumes" optional:"true"`
	}

	resourceSetAction struct {
		desc           string
		step           spec.StepInstance
		cfg            *ResourceSetConfig
		keyJQ          *JQCheck
		orphanFilterJQ *JQCheck
		items          []map[string]any
		redact         []compiledRedact
	}
)

func (ResourceSet) Kind() string   { return "rest.resource_set" }
func (ResourceSet) NewConfig() any { return &ResourceSetConfig{} }

func (c *ResourceSetConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (c *ResourceSetConfig) RefMaps() []map[string]any {
	var maps []map[string]any
	for _, item := range c.Items {
		if m, ok := item.(map[string]any); ok {
			maps = append(maps, m)
		}
	}
	if c.OrphanState != nil {
		maps = append(maps, c.OrphanState)
	}
	return maps
}

func (ResourceSet) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*ResourceSetConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &ResourceSetConfig{}, step.Config)
	}

	// bare-error: plan-time validation, surfaced via scampi source span
	invalid := func(format string, args ...any) error { return errs.Errorf(format, args...) }

	if cfg.Query == nil {
		return nil, invalid("rest.resource_set: query is required")
	}
	if cfg.Query.Check == nil {
		return nil, invalid("rest.resource_set: query must have a check (use rest.jq)")
	}
	if _, ok := cfg.Query.Check.(*JQCheck); !ok {
		return nil, invalid(
			"rest.resource_set: query check must be rest.jq, got %s",
			cfg.Query.Check.Kind(),
		)
	}
	if cfg.Key == nil {
		return nil, invalid("rest.resource_set: key is required")
	}
	keyJQ, ok := cfg.Key.(*JQCheck)
	if !ok {
		return nil, invalid(
			"rest.resource_set: key must be rest.jq, got %s",
			cfg.Key.Kind(),
		)
	}
	// Convert []any items to []map[string]any at plan time.
	var items []map[string]any
	for i, raw := range cfg.Items {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, invalid("rest.resource_set: items[%d] must be a map, got %T", i, raw)
		}
		items = append(items, m)
	}
	if len(items) == 0 && cfg.Orphan == nil {
		return nil, invalid("rest.resource_set: items is empty and no orphan handler — nothing to reconcile")
	}
	if cfg.Missing == nil && cfg.Found == nil && cfg.Orphan == nil {
		return nil, invalid("rest.resource_set: at least one of missing, found, or orphan is required")
	}
	if len(cfg.Bindings) > 0 && cfg.Found == nil && cfg.Orphan == nil {
		return nil, invalid("rest.resource_set: bindings require a found or orphan request")
	}
	var orphanFilterJQ *JQCheck
	if cfg.OrphanFilter != nil {
		var ok bool
		orphanFilterJQ, ok = cfg.OrphanFilter.(*JQCheck)
		if !ok {
			return nil, invalid(
				"rest.resource_set: orphan_filter must be rest.jq, got %s",
				cfg.OrphanFilter.Kind(),
			)
		}
	}

	redact, err := compileRedact(cfg.Query.Redact, step.Source)
	if err != nil {
		return nil, err
	}

	return &resourceSetAction{
		desc:           cfg.Desc,
		step:           step,
		cfg:            cfg,
		keyJQ:          keyJQ,
		orphanFilterJQ: orphanFilterJQ,
		items:          items,
		redact:         redact,
	}, nil
}

func (a *resourceSetAction) Desc() string { return a.desc }
func (a *resourceSetAction) Kind() string { return "rest.resource_set" }

func (a *resourceSetAction) Ops() []spec.Op {
	op := &resourceSetOp{
		query:        a.cfg.Query,
		keyJQ:        a.keyJQ,
		orphanFilter: a.orphanFilterJQ,
		items:        a.items,
		missing:      a.cfg.Missing,
		found:        a.cfg.Found,
		orphan:       a.cfg.Orphan,
		bindings:     a.cfg.Bindings,
		orphanState:  a.cfg.OrphanState,
		redact:       a.redact,
	}
	op.SetAction(a)
	return []spec.Op{op}
}

// StepID returns this action's step ID for output registry keying.
func (a *resourceSetAction) StepID() spec.StepID { return a.step.ID }

// Promiser
// -----------------------------------------------------------------------------

func (a *resourceSetAction) Promises() []spec.Resource {
	return []spec.Resource{spec.RefResource(a.step.ID)}
}

func (a *resourceSetAction) Inputs() []spec.Resource {
	var refs []spec.Resource
	for _, m := range a.cfg.RefMaps() {
		refs = append(refs, collectRefs(m)...)
	}
	return refs
}

func (a *resourceSetAction) ResolveRefs(resolve spec.RefResolver) error {
	for _, m := range a.cfg.RefMaps() {
		if err := resolveMapRefs(m, resolve); err != nil {
			return err
		}
	}
	return nil
}
