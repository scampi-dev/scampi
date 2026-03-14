// SPDX-License-Identifier: GPL-3.0-only

package firewall

import (
	"fmt"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

// BackendNotFoundError is emitted when neither ufw nor firewall-cmd is found.
type BackendNotFoundError struct{}

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

func (BackendNotFoundError) Severity() signal.Severity { return signal.Error }
func (BackendNotFoundError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// RuleCheckError is emitted when checking rule existence fails.
type RuleCheckError struct {
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

func (RuleCheckError) Severity() signal.Severity { return signal.Error }
func (RuleCheckError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// RuleApplyError is emitted when applying a firewall rule fails.
type RuleApplyError struct {
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

func (RuleApplyError) Severity() signal.Severity { return signal.Error }
func (RuleApplyError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// InvalidPortError is returned during validation when the port format is invalid.
type InvalidPortError struct {
	Port   string
	Source spec.SourceSpan
}

func (e InvalidPortError) Error() string {
	return fmt.Sprintf("invalid port %q", e.Port)
}

func (e InvalidPortError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.firewall.InvalidPort",
		Text:   `invalid port "{{.Port}}"`,
		Hint:   `use <port>/<proto> or <start>:<end>/<proto>, e.g. "22/tcp", "53/udp", "6000:6007/tcp"`,
		Data:   e,
		Source: &e.Source,
	}
}

func (InvalidPortError) Severity() signal.Severity { return signal.Error }
func (InvalidPortError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// InvalidActionError is returned during validation when the action is not recognized.
type InvalidActionError struct {
	Action string
	Source spec.SourceSpan
}

func (e InvalidActionError) Error() string {
	return fmt.Sprintf("invalid action %q", e.Action)
}

func (e InvalidActionError) EventTemplate() event.Template {
	return event.Template{
		ID:     "builtin.firewall.InvalidAction",
		Text:   `invalid action "{{.Action}}"`,
		Hint:   `use one of: "allow", "deny", "reject"`,
		Data:   e,
		Source: &e.Source,
	}
}

func (InvalidActionError) Severity() signal.Severity { return signal.Error }
func (InvalidActionError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
