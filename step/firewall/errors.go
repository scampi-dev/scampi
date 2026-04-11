// SPDX-License-Identifier: GPL-3.0-only

package firewall

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/spec"
)

// BackendNotFoundError is emitted when neither ufw nor firewall-cmd is found.
type BackendNotFoundError struct {
	diagnostic.FatalError
}

func (e BackendNotFoundError) Error() string {
	return "no supported firewall backend found"
}

func (e BackendNotFoundError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.firewall.BackendNotFound",
		Text: "no supported firewall backend found",
		Hint: "install ufw or firewalld on the target",
		Data: e,
	}
}

// RuleCheckError is emitted when checking rule existence fails.
type RuleCheckError struct {
	diagnostic.FatalError
	Port   string
	Stderr string
}

func (e RuleCheckError) Error() string {
	return fmt.Sprintf("firewall rule check for %s failed: %s", e.Port, e.Stderr)
}

func (e RuleCheckError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.firewall.RuleCheckFailed",
		Text: `failed to check firewall rule for port {{.Port}}`,
		Hint: `stderr: {{.Stderr}}`,
		Data: e,
	}
}

// RuleApplyError is emitted when applying a firewall rule fails.
type RuleApplyError struct {
	diagnostic.FatalError
	Port   string
	Action string
	Stderr string
}

func (e RuleApplyError) Error() string {
	return fmt.Sprintf("firewall %s %s failed: %s", e.Action, e.Port, e.Stderr)
}

func (e RuleApplyError) EventTemplate() event.Template {
	return event.Template{
		ID:   "builtin.firewall.RuleApplyFailed",
		Text: `failed to {{.Action}} port {{.Port}}`,
		Hint: `stderr: {{.Stderr}}`,
		Data: e,
	}
}

// InvalidPortError is returned during validation when the port format is invalid.
type InvalidPortError struct {
	diagnostic.FatalError
	Port   string
	Detail string
	Source spec.SourceSpan
}

func (e InvalidPortError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("invalid port %q: %s", e.Port, e.Detail)
	}
	return fmt.Sprintf("invalid port %q", e.Port)
}

func (e InvalidPortError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.firewall.InvalidPort",
		Text:   `invalid port "{{.Port}}": {{.Detail}}`,
		Hint:   `use <port>/<proto> or <start>:<end>/<proto>, e.g. "22/tcp", "53/udp", "6000:6007/tcp"`,
		Data:   e,
		Source: &e.Source,
	}
}
