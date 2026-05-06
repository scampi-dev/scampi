// SPDX-License-Identifier: GPL-3.0-only

package firewall

import (
	"strconv"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// FirewallPort represents a port/protocol specification.
type FirewallPort struct {
	Port    int              // single port or range start
	EndPort int              // range end, 0 for single port
	Proto   target.PortProto // TCP or UDP
}

// String reconstructs the port spec, e.g. "22/tcp" or "6000:6007/tcp".
func (p FirewallPort) String() string {
	if p.EndPort != 0 {
		return strconv.Itoa(p.Port) + ":" + strconv.Itoa(p.EndPort) + "/" + p.Proto.String()
	}
	return strconv.Itoa(p.Port) + "/" + p.Proto.String()
}

// Action represents a firewall rule action.
type Action uint8

const (
	ActionAllow Action = iota + 1
	ActionDeny
	ActionReject
)

// ActionValues is the exhaustive list of accepted action strings.
var ActionValues = []string{"allow", "deny", "reject"}

// ProtoValues is the exhaustive list of accepted protocol strings.
var ProtoValues = []string{"tcp", "udp"}

func (a Action) String() string {
	switch a {
	case ActionAllow:
		return "allow"
	case ActionDeny:
		return "deny"
	case ActionReject:
		return "reject"
	default:
		return "unknown"
	}
}

func parseAction(s string) Action {
	switch s {
	case "allow":
		return ActionAllow
	case "deny":
		return ActionDeny
	case "reject":
		return ActionReject
	default:
		panic(errs.BUG("invalid firewall action %q — should have been caught by validate", s))
	}
}

func parseProto(s string) target.PortProto {
	switch s {
	case "tcp":
		return target.ProtoTCP
	case "udp":
		return target.ProtoUDP
	default:
		panic(errs.BUG("invalid protocol %q — should have been caught by validate", s))
	}
}

type (
	Firewall       struct{}
	FirewallConfig struct {
		_ struct{} `summary:"Manage firewall rules via UFW or firewalld"`

		Desc     string   `step:"Human-readable description" optional:"true"`
		Port     int      `step:"Port number" example:"8080"`
		EndPort  int      `step:"End of port range (for ranges)" optional:"true" example:"9000"`
		Proto    string   `step:"Protocol" default:"tcp" example:"udp"`
		Action   string   `step:"Rule action" default:"allow" example:"allow|deny|reject"`
		Promises []string `step:"Cross-deploy resources this step produces" optional:"true"`
		Inputs   []string `step:"Cross-deploy resources this step consumes" optional:"true"`
	}
	firewallAction struct {
		desc   string
		port   FirewallPort
		action Action
		step   spec.StepInstance
	}
)

func (Firewall) Kind() string   { return "firewall" }
func (Firewall) NewConfig() any { return &FirewallConfig{} }

func (c *FirewallConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (*FirewallConfig) FieldEnumValues() map[string][]string {
	return map[string][]string{
		"action": ActionValues,
		"proto":  ProtoValues,
	}
}

func (Firewall) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*FirewallConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &FirewallConfig{}, step.Config)
	}

	// @min/@max on the stub enforce 1-65535 for literal arguments.
	// Dynamic expressions bypass that, so guard here too.
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, PortOutOfRangeError{
			Field: "port", Value: cfg.Port,
			Source: step.Fields["port"].Value,
		}
	}
	if cfg.EndPort != 0 {
		if cfg.EndPort < 1 || cfg.EndPort > 65535 {
			return nil, PortOutOfRangeError{
				Field: "end_port", Value: cfg.EndPort,
				Source: step.Fields["end_port"].Value,
			}
		}
		if cfg.EndPort <= cfg.Port {
			return nil, InvalidRangeError{
				Port: cfg.Port, EndPort: cfg.EndPort,
				Source: step.Fields["end_port"].Value,
			}
		}
	}

	port := FirewallPort{
		Port:    cfg.Port,
		EndPort: cfg.EndPort,
		Proto:   parseProto(cfg.Proto),
	}

	return &firewallAction{
		desc:   cfg.Desc,
		port:   port,
		action: parseAction(cfg.Action),
		step:   step,
	}, nil
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
