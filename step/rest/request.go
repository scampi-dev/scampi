// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

type (
	Request       struct{}
	RequestConfig struct {
		_ struct{} `summary:"Make an HTTP request against a REST target"`

		Desc    string            `step:"Human-readable description" optional:"true"`
		Method  string            `step:"HTTP method" example:"POST"`
		Path    string            `step:"Request path" example:"/nginx/proxy-hosts"`
		Headers map[string]string `step:"HTTP headers" optional:"true"`
		Body    BodyConfig        `step:"Request body" optional:"true"`
		Check   CheckConfig       `step:"Check matcher for idempotency" optional:"true"`
	}

	requestAction struct {
		desc   string
		method string
		path   string
		step   spec.StepInstance
		cfg    *RequestConfig
	}
)

func (Request) Kind() string   { return "rest.request" }
func (Request) NewConfig() any { return &RequestConfig{} }

func (Request) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*RequestConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &RequestConfig{}, step.Config)
	}

	return &requestAction{
		desc:   cfg.Desc,
		method: cfg.Method,
		path:   cfg.Path,
		step:   step,
		cfg:    cfg,
	}, nil
}

func (a *requestAction) Desc() string { return a.desc }
func (a *requestAction) Kind() string { return "rest.request" }

func (a *requestAction) Ops() []spec.Op {
	op := &requestOp{
		method:  a.method,
		path:    a.path,
		headers: a.cfg.Headers,
		body:    a.cfg.Body,
		check:   a.cfg.Check,
	}
	op.SetAction(a)
	return []spec.Op{op}
}
