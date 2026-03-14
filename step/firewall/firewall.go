// SPDX-License-Identifier: GPL-3.0-only

package firewall

import (
	"regexp"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

var portRe = regexp.MustCompile(`^(\d+)(:\d+)?/(tcp|udp)$`)

type (
	Firewall       struct{}
	FirewallConfig struct {
		_ struct{} `summary:"Manage firewall rules via UFW or firewalld"`

		Desc   string `step:"Human-readable description" optional:"true"`
		Port   string `step:"Port/protocol string" example:"22/tcp"`
		Action string `step:"Rule action" default:"allow" example:"allow|deny|reject"`
	}
	firewallAction struct {
		idx    int
		desc   string
		port   string
		action string
		step   spec.StepInstance
	}
)

func (Firewall) Kind() string   { return "firewall" }
func (Firewall) NewConfig() any { return &FirewallConfig{} }

func (Firewall) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*FirewallConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &FirewallConfig{}, step.Config)
	}

	if err := cfg.validate(step); err != nil {
		return nil, err
	}

	return &firewallAction{
		idx:    idx,
		desc:   cfg.Desc,
		port:   cfg.Port,
		action: cfg.Action,
		step:   step,
	}, nil
}

func (c *FirewallConfig) validate(step spec.StepInstance) error {
	if !portRe.MatchString(c.Port) {
		return InvalidPortError{
			Port:   c.Port,
			Source: step.Fields["port"].Value,
		}
	}

	switch c.Action {
	case "allow", "deny", "reject":
	default:
		return InvalidActionError{
			Action: c.Action,
			Source: step.Fields["action"].Value,
		}
	}

	return nil
}

func (a *firewallAction) Desc() string { return a.desc }
func (a *firewallAction) Kind() string { return "firewall" }

func (a *firewallAction) Ops() []spec.Op {
	op := &ensureRuleOp{
		port:   a.port,
		action: a.action,
	}
	op.SetAction(a)
	return []spec.Op{op}
}
