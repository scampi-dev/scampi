// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=EngineKind
//go:generate stringer -type=PlanKind
//go:generate stringer -type=ActionKind
//go:generate stringer -type=OpKind
//go:generate stringer -type=Chattiness
//go:generate stringer -type=Severity
//go:generate stringer -type=CauseKind
//go:generate stringer -type=ChangePhase
package event

import (
	"time"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

type StepDetail struct {
	StepIndex int
	StepKind  string
	StepDesc  string
	HookID    string // non-empty when this op belongs to a hook
}

type EngineEvent struct {
	Time             time.Time
	Kind             EngineKind
	Detail           *EngineFinishedDetail
	ConnectingDetail *EngineConnectingDetail
	Severity         signal.Severity
	Chattiness       Chattiness
}

type EngineConnectingDetail struct {
	TargetName string
	TargetKind string
}

type PlanEvent struct {
	Time           time.Time
	Kind           PlanKind
	Step           StepDetail
	StartedDetail  *PlanStartedDetail
	Detail         *PlanDetail
	FinishedDetail *PlanFinishedDetail
	Severity       signal.Severity
	Chattiness     Chattiness
}

type ActionEvent struct {
	Time       time.Time
	Kind       ActionKind
	Step       StepDetail
	Detail     *ActionDetail
	HookDetail *HookDetail
	Severity   signal.Severity
	Chattiness Chattiness
}

type OpEvent struct {
	Time          time.Time
	Kind          OpKind
	Step          StepDetail
	DisplayID     string
	CheckDetail   *OpCheckDetail
	ExecuteDetail *OpExecuteDetail
	Severity      signal.Severity
	Chattiness    Chattiness
}

type IndexAllEvent struct {
	Time       time.Time
	Steps      []StepIndexDetail
	Severity   signal.Severity
	Chattiness Chattiness
}

type IndexStepEvent struct {
	Time       time.Time
	Doc        spec.StepDoc
	Severity   signal.Severity
	Chattiness Chattiness
}

// GraphEvent surfaces the cross-deploy resource graph topology
// before any plan starts. Emitted once per scampi run when the
// graph is non-trivial (more than one level OR any explicit dep);
// suppressed for the single-deploy / all-roots case where there's
// nothing structural to show. See #276.
type GraphEvent struct {
	Time       time.Time
	Detail     GraphDetail
	Severity   signal.Severity
	Chattiness Chattiness
}

type GraphDetail struct {
	Levels []GraphLevel
}

type GraphLevel struct {
	Index int
	Nodes []GraphNode
}

type GraphNode struct {
	DeployName string
	TargetName string
	// After lists the deploy names this node waits on (empty for roots).
	After []string
	// Needs lists the resource names that drove the dep edges
	// (e.g. "lxc:1000", "label:realm:skrynet.lan"). Empty for roots
	// or for nodes whose only deps were external inputs.
	Needs []string
}

type InspectEvent struct {
	Time       time.Time
	Detail     InspectDetail
	Severity   signal.Severity
	Chattiness Chattiness
}

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

type EngineDiagnostic struct {
	Time       time.Time
	CfgPath    string
	Detail     DiagnosticDetail
	Severity   signal.Severity
	Chattiness Chattiness
}

type PlanDiagnostic struct {
	Time       time.Time
	Step       StepDetail
	Detail     DiagnosticDetail
	Severity   signal.Severity
	Chattiness Chattiness
}

type ActionDiagnostic struct {
	Time       time.Time
	Step       StepDetail
	Detail     DiagnosticDetail
	Severity   signal.Severity
	Chattiness Chattiness
}

type OpDiagnostic struct {
	Time       time.Time
	Step       StepDetail
	DisplayID  string
	Detail     DiagnosticDetail
	Severity   signal.Severity
	Chattiness Chattiness
}

type EngineKind uint8

const (
	EngineStarted EngineKind = iota
	EngineConnecting
	EngineFinished
)

type PlanKind uint8

const (
	PlanStarted PlanKind = iota
	PlanFinished
	StepPlanned
	PlanProduced
)

type ActionKind uint8

const (
	ActionStarted ActionKind = iota
	ActionFinished
	HookTriggered
	HookSkipped
)

type OpKind uint8

const (
	OpCheckStarted OpKind = iota
	OpChecked

	OpExecuteStarted
	OpExecuted
)

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

// New diagnostic surface
// =============================================================================
// These types replace the per-scope {Engine,Plan,Action,Op}{Event,Diagnostic}
// envelopes above. They are introduced additively during the rework; the old
// types stay in place until the final phase, at which point the surface
// above this banner is deleted. See doc/design/diagnostics.md.

// Severity collapses signal.Severity (6 levels) to the 3 levels users
// actually need. The old enum stays alive until phase 6 because the
// surviving lifecycle events still reference it.
type Severity uint8

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

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

// Diagnostic is the unified diagnostic event. It replaces the four
// scope-specific envelope types (EngineDiagnostic, PlanDiagnostic,
// ActionDiagnostic, OpDiagnostic). Source location lives on
// Template.Source; there is no separate scope axis.
type Diagnostic struct {
	Time     time.Time
	Severity Severity
	Template Template
	Cause    Cause
}

// ChangePhase distinguishes a would-change (check-only or pre-execute)
// from a did-change (post-execute). Same shape both phases.
type ChangePhase uint8

const (
	ChangePlanned ChangePhase = iota
	ChangeExecuted
)

// StepRef identifies the step a Change belongs to. The fields mirror
// the existing StepDetail but live here so phase 6 can delete
// StepDetail cleanly.
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

// Progress is a status update - "currently doing X". Latest-wins on
// TTY (the consumer overwrites a status line); appended one line at a
// time on non-TTY. No severity, no cause: too ephemeral to bother.
type Progress struct {
	Time time.Time
	Text string
}
