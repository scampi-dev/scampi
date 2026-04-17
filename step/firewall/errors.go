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
		ID:   CodeBackendNotFound,
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
		ID:   CodeRuleCheckFailed,
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
		ID:   CodeRuleApplyFailed,
		Text: `failed to {{.Action}} port {{.Port}}`,
		Hint: `stderr: {{.Stderr}}`,
		Data: e,
	}
}

// PortOutOfRangeError is returned when a port number is outside 1-65535.
type PortOutOfRangeError struct {
	diagnostic.FatalError
	Field  string
	Value  int
	Source spec.SourceSpan
}

func (e PortOutOfRangeError) Error() string {
	return fmt.Sprintf("%s %d is out of range (1–65535)", e.Field, e.Value)
}

func (e PortOutOfRangeError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodePortOutOfRange,
		Text:   `{{.Field}} {{.Value}} is out of range`,
		Hint:   "port numbers must be between 1 and 65535",
		Data:   e,
		Source: &e.Source,
	}
}

// InvalidRangeError is returned when end_port <= port.
type InvalidRangeError struct {
	diagnostic.FatalError
	Port    int
	EndPort int
	Source  spec.SourceSpan
}

func (e InvalidRangeError) Error() string {
	return fmt.Sprintf("end_port %d must be greater than port %d", e.EndPort, e.Port)
}

func (e InvalidRangeError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeInvalidRange,
		Text:   `end_port {{.EndPort}} must be greater than port {{.Port}}`,
		Hint:   "end_port defines the upper bound of a port range",
		Data:   e,
		Source: &e.Source,
	}
}
