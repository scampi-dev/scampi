// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=Impact
//go:generate stringer -type=CauseKind
//go:generate stringer -type=ChangePhase
package event

import (
	"time"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
)

// InspectDetail
// -----------------------------------------------------------------------------

type InspectDetail struct {
	DeployName string
	TargetName string
	Entries    []InspectEntry
}

type InspectEntry struct {
	Index  int
	Kind   string
	Desc   string
	Fields []spec.InspectField
}

// Template
// -----------------------------------------------------------------------------

// TemplateOf extracts the Template from any diagnostic-shaped event
// (Error / Warning / Info). Returns the zero Template for events that
// don't carry one (Change / Progress).
func TemplateOf(ev Event) Template {
	switch v := ev.(type) {
	case Error:
		return v.Template
	case Warning:
		return v.Template
	case Info:
		return v.Template
	default:
		return Template{}
	}
}

type Template struct {
	ID   errs.Code
	Text string
	Hint string
	Help string
	Data any

	Source *spec.SourceSpan
}

// Field is a single renderable text within a Template.
type Field struct {
	id, text string
	data     any
}

func (f Field) TemplateID() string   { return f.id }
func (f Field) TemplateText() string { return f.text }
func (f Field) TemplateData() any    { return f.data }

func (t Template) TextField() Field { return Field{string(t.ID) + ".Text", t.Text, t.Data} }
func (t Template) HintField() Field { return Field{string(t.ID) + ".Hint", t.Hint, t.Data} }
func (t Template) HelpField() Field { return Field{string(t.ID) + ".Help", t.Help, t.Data} }

// Cause
// -----------------------------------------------------------------------------

// CauseKind identifies what triggered an event. Most events have no
// notable trigger (CauseNone); hooks are the first thing that does.
// Grow this enum as new triggers appear (deferred resource arrival,
// scheduled re-eval, retry context, ...).
type CauseKind uint8

const (
	CauseNone CauseKind = iota
	CauseHook
)

// Cause is the optional "what triggered this event" tag. Value type:
// the zero value (Cause{}) means "no notable trigger" and avoids
// allocating for the common case.
type Cause struct {
	Kind CauseKind
	Ref  string // hook ID for CauseHook; empty otherwise
}

// Event is the sealed union of everything Emit accepts: the
// diagnostics (Error, Warning, Info) and the streaming events
// (Change, Progress). External types cannot join the union - the
// sealing method isEvent is unexported.
type Event interface{ isEvent() }

// Impact lives on Error only. Warning and Info never abort execution.
type Impact uint8

const (
	ImpactNone Impact = iota
	ImpactAbort
)

func (i Impact) ShouldAbort() bool { return i == ImpactAbort }

// Error is a diagnostic that may or may not abort, depending on Impact.
// Producers return a value of this type from their Diagnostic() method;
// the engine reads .Impact to decide whether execution stops.
type Error struct {
	Time     time.Time
	Impact   Impact
	Template Template
	Cause    Cause
}

// Warning is a non-fatal diagnostic advisory. Never aborts.
type Warning struct {
	Time     time.Time
	Template Template
	Cause    Cause
}

// Info is an informational diagnostic. Never aborts.
type Info struct {
	Time     time.Time
	Template Template
	Cause    Cause
}

func (Error) isEvent()    {}
func (Warning) isEvent()  {}
func (Info) isEvent()     {}
func (Change) isEvent()   {}
func (Progress) isEvent() {}

// Change
// -----------------------------------------------------------------------------

// ChangePhase distinguishes a would-change (check-only or pre-execute)
// from a did-change (post-execute). Same shape both phases.
type ChangePhase uint8

const (
	ChangePlanned ChangePhase = iota
	ChangeExecuted
)

// StepRef identifies the step a Change belongs to.
type StepRef struct {
	Index int
	Kind  string
	Desc  string
}

// Change is a planned or executed mutation reported by an op. Emitted
// live as drift is detected (check) or as the mutation happens (apply).
type Change struct {
	Time      time.Time
	Phase     ChangePhase
	Step      StepRef
	DisplayID string
	Drift     spec.DriftDetail
	Cause     Cause
}

// Progress
// -----------------------------------------------------------------------------

// Progress is a status update - "currently doing X". Latest-wins on
// TTY (the consumer overwrites a status line); appended one line at a
// time on non-TTY. No severity, no cause: too ephemeral to bother.
type Progress struct {
	Time time.Time
	Text string
}

// PlanDetail
// -----------------------------------------------------------------------------

// PlanDetail / PlannedAction / PlannedOp model the rendered structure of
// `scampi plan` output: actions in order, each carrying their ops and
// inter-op dependency edges. Returned from engine.Plan() rather than
// emitted as an event.
type PlanDetail struct {
	DeployID   string
	DeployDesc string
	Actions    []PlannedAction
}

type PlannedAction struct {
	Index     int
	Desc      string
	Kind      string
	DependsOn []int
	Ops       []PlannedOp
}

type PlannedOp struct {
	Index     int
	DisplayID string
	DependsOn []int
	Template  *spec.PlanTemplate // nil = no template, use DisplayID
}
