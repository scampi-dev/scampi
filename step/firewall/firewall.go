// SPDX-License-Identifier: GPL-3.0-only

package firewall

import (
	"strconv"
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// FirewallPort represents a parsed port/protocol specification.
type FirewallPort struct {
	Port    string           // single port or range start
	EndPort string           // range end, empty for single port
	Proto   target.PortProto // TCP or UDP
}

// String reconstructs the original port spec, e.g. "22/tcp" or "6000:6007/tcp".
func (p FirewallPort) String() string {
	if p.EndPort != "" {
		return p.Port + ":" + p.EndPort + "/" + p.Proto.String()
	}
	return p.Port + "/" + p.Proto.String()
}

func ParseFirewallPort(s string) (FirewallPort, error) {
	portPart, proto, ok := strings.Cut(s, "/")
	if !ok {
		return FirewallPort{}, portParseError("missing protocol suffix")
	}
	var p target.PortProto
	switch proto {
	case "tcp":
		p = target.ProtoTCP
	case "udp":
		p = target.ProtoUDP
	default:
		return FirewallPort{}, portParseError("unsupported protocol \"" + proto + "\"")
	}

	start, end, isRange := strings.Cut(portPart, ":")
	if err := validatePortNumber(start); err != nil {
		return FirewallPort{}, err
	}
	if isRange {
		if err := validatePortNumber(end); err != nil {
			return FirewallPort{}, err
		}
	}

	fp := FirewallPort{Port: start, Proto: p}
	if isRange {
		fp.EndPort = end
	}
	return fp, nil
}

func validatePortNumber(s string) error {
	if s == "" {
		return portParseError("empty port number")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return portParseError("\"" + s + "\" is not a number")
	}
	if n < 0 || n > 65535 {
		return portParseError(strconv.Itoa(n) + " is out of range (0–65535)")
	}
	return nil
}

type portParseError string

func (e portParseError) Error() string { return string(e) }

// Action represents a firewall rule action.
type Action uint8

const (
	ActionAllow Action = iota + 1
	ActionDeny
	ActionReject
)

// ActionValues is the exhaustive list of accepted action strings.
var ActionValues = []string{"allow", "deny", "reject"}

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

type (
	Firewall       struct{}
	FirewallConfig struct {
		_ struct{} `summary:"Manage firewall rules via UFW or firewalld"`

		Desc   string `step:"Human-readable description" optional:"true"`
		Port   string `step:"Port/protocol string" example:"22/tcp"`
		Action string `step:"Rule action" default:"allow" example:"allow|deny|reject"`
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

func (*FirewallConfig) FieldEnumValues() map[string][]string {
	return map[string][]string{
		"action": ActionValues,
	}
}

func (Firewall) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*FirewallConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &FirewallConfig{}, step.Config)
	}

	// Action is a typed enum in the stub — lang/check enforces.
	// Port format is partially validated by @std.pattern; the
	// parser still runs because the runtime needs the structured
	// FirewallPort value, and only ParseFirewallPort knows how to
	// extract Port/EndPort/Proto from the string. A parse error
	// reaching this point means a non-literal port expression
	// bypassed the static check — surface the same typed error.
	port, parseErr := ParseFirewallPort(cfg.Port)
	if parseErr != nil {
		return nil, InvalidPortError{
			Port:   cfg.Port,
			Detail: parseErr.Error(),
			Source: step.Fields["port"].Value,
		}
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
