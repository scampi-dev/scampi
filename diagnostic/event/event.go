//go:generate stringer -type=Kind
//go:generate stringer -type=Scope
//go:generate stringer -type=Chattiness
package event

import (
	"time"

	"godoit.dev/doit/signal"
)

// Event represents a single, immutable fact that occurred during execution.
//
// An Event describes *what happened*, not how it should be rendered.
// It is the primary integration point between the engine, diagnostics,
// policy, and renderers.
//
// Invariant:
//   - Every Event MUST be emitted with Severity and Chattiness fully populated.
//   - Renderers MUST NOT infer or guess defaults for either field.
//   - Policy MAY adjust Severity, but MUST NOT alter Chattiness.
//   - Chattiness MUST NOT be used to indicate importance or failure.
//
// These rules ensure that events are semantically complete at creation time
// and can be safely consumed by multiple renderers (CLI, JSON, UI) without
// hidden coupling or duplicated logic.
type Event struct {
	Time       time.Time
	Kind       Kind
	Scope      Scope
	Subject    Subject
	Detail     any
	Severity   signal.Severity
	Chattiness Chattiness
}

type Kind uint8

const (
	EngineStarted Kind = iota
	EngineFinished
	PlanStarted
	PlanFinished
	UnitPlanned

	ActionStarted
	ActionFinished

	OpCheckStarted
	OpChecked

	OpExecuteStarted
	OpExecuted

	DiagnosticRaised
)

type Scope uint8

const (
	ScopeEngine Scope = iota
	ScopePlan
	ScopeAction
	ScopeOp
)

type Subject struct {
	Action string
	Op     string
	Index  int
	Kind   string
	Name   string
}

// Chattiness describes how noisy an event is under normal operation.
// It is orthogonal to Severity and MUST NOT be used to indicate importance.
type Chattiness uint8

const (
	Subtle Chattiness = iota
	Reserved
	Normal
	Chatty
	Yappy
)

type Template struct {
	ID   string
	Text string
	Hint string
	Help string
	Data any
}
