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
// (Change, Progress, Begin, Result). External types cannot join the
// union - the sealing method isEvent is unexported.
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
func (Begin) isEvent()    {}
func (Result) isEvent()   {}

// Change
// -----------------------------------------------------------------------------

// ChangePhase distinguishes a would-change (check-only or pre-execute)
// from a did-change (post-execute). Same shape both phases.
type ChangePhase uint8

const (
	ChangePlanned ChangePhase = iota
	ChangeExecuted
)

// DeployRef identifies the deploy lane an event belongs to. Ordinal is unique
// per lane in a run (assigned level-major, declaration order within a level)
// and keys the render Sequencer's per-deploy cursor. Name is the deploy block's
// name, set only when a run has more than one lane so single-deploy output stays
// untagged; an empty Name means "do not tag".
type DeployRef struct {
	Name    string
	Ordinal int
}

// StepRef identifies the step a Change belongs to, within its deploy lane.
type StepRef struct {
	Deploy DeployRef
	Index  int
	Kind   string
	Desc   string
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

// Progress reports position through a run: Completed of Total work units, with
// Current naming the step in flight. Total == 0 means indeterminate (no count
// to show). No severity, no cause: too ephemeral to bother. Real Completed/
// Total counting lands with the scheduler progress hook; the type is defined
// now so the stream sink can render it (#430).
type Progress struct {
	Time      time.Time
	Total     int
	Completed int
	Current   StepRef
}

// Result
// -----------------------------------------------------------------------------

// StepOutcome is the overall verdict for one step, derived from its op summary.
type StepOutcome uint8

const (
	StepUnchanged StepOutcome = iota // all ops satisfied / skipped
	StepChanged                      // at least one op changed (apply) or would (check)
	StepFailed                       // at least one op failed or aborted
)

// StepSummary is the per-op count breakdown for a finished step. Field-
// identical to the engine's step summary, copied onto the event so the
// stream sink never reaches back into the execution report.
type StepSummary struct {
	Total       int
	Succeeded   int
	Failed      int
	Aborted     int
	Skipped     int
	Changed     int
	WouldChange int
}

// Begin marks a step entering execution: the live-region counterpart to Result
// (the finish). Naming is begin/finish, never start. Emitted before a step's ops
// run; the render layer pairs each Begin with its Result to track the in-flight
// set and per-deploy progress. Out-of-band like Progress: it is a transient
// signal for the live region, not part of the durable ordered record.
type Begin struct {
	Time  time.Time
	Step  StepRef
	Cause Cause
}

// Result is the completion of one step: emitted on the stream as each step
// settles (check or apply), distinct from Progress (position) and Change
// (per-field mutation). It carries the step's verdict and op breakdown.
type Result struct {
	Time    time.Time
	Step    StepRef
	Outcome StepOutcome
	Summary StepSummary
	Cause   Cause
}
