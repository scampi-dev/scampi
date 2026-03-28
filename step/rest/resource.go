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

func (Resource) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*ResourceConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &ResourceConfig{}, step.Config)
	}

	// bare-error: plan-time validation, surfaced via Starlark source span
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
