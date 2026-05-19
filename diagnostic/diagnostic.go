// SPDX-License-Identifier: GPL-3.0-only

//go:generate stringer -type=Impact
package diagnostic

import (
	"reflect"
	"strings"
	"time"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

// OpDisplayID derives a display identifier for an op.
// Uses OpDescriber template ID if available, otherwise falls back to type name.
func OpDisplayID(op spec.Op) string {
	if d, ok := op.(spec.OpDescriber); ok {
		if desc := d.OpDescription(); desc != nil {
			if id := desc.PlanTemplate().ID; id != "" {
				return string(id)
			}
		}
	}
	// Fallback: use the struct type name
	t := reflect.TypeOf(op)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

type Impact uint8

const (
	ImpactNone Impact = iota
	ImpactAbort
)

func (i Impact) ShouldAbort() bool {
	return i == ImpactAbort
}

// Deferrable is implemented by errors that reference a missing resource which
// could be created by an upstream action. The engine uses this to defer aborts
// during check mode when the resource is already promised.
type Deferrable interface {
	DeferredResource() spec.Resource
}

type (
	Emitter interface {
		EmitGraph(e event.GraphEvent)

		// Legacy diagnostic envelopes. Producers migrate to
		// EmitDiagnostic during phase 5 (bare-error migration); the
		// four envelope methods disappear with the migration.
		EmitEngineDiagnostic(e event.EngineDiagnostic)
		EmitPlanDiagnostic(e event.PlanDiagnostic)
		EmitActionDiagnostic(e event.ActionDiagnostic)
		EmitOpDiagnostic(e event.OpDiagnostic)

		EmitDiagnostic(e event.Diagnostic)
		EmitChange(e event.Change)
		EmitProgress(e event.Progress)
	}
	Diagnostic interface {
		EventTemplate() event.Template
		Severity() signal.Severity
		Impact() Impact
	}
	// FatalError provides the Severity and Impact methods shared by all
	// diagnostic errors that abort execution: Severity=Error, Impact=Abort.
	// Embed it in error structs to avoid repeating these two methods.
	FatalError struct{}

	// Warning provides Severity=Warning, Impact=None for non-fatal diagnostics
	// that should be reported but don't stop execution.
	Warning struct{}

	// Info provides Severity=Info, Impact=None for informational diagnostics.
	Info struct{}

	Diagnostics     []Diagnostic
	MultiDiagnostic interface {
		Diagnostics() []Diagnostic
	}
)

func (FatalError) Severity() signal.Severity { return signal.Error }
func (FatalError) Impact() Impact            { return ImpactAbort }

func (Warning) Severity() signal.Severity { return signal.Warning }
func (Warning) Impact() Impact            { return ImpactNone }

func (Info) Severity() signal.Severity { return signal.Info }
func (Info) Impact() Impact            { return ImpactNone }

func (d Diagnostics) Diagnostics() []Diagnostic { return d }
func (d Diagnostics) Error() string {
	msgs := make([]string, len(d))
	for i, diag := range d {
		if e, ok := diag.(error); ok {
			msgs[i] = e.Error()
		} else {
			msgs[i] = diag.EventTemplate().Text
		}
	}
	return strings.Join(msgs, "; ")
}

// ActionDeps maps action index to indices of actions it depends on.
type ActionDeps [][]int

// PlanProduced constructs the event payload for `scampi plan` output -
// a structural view of the planned actions, their ops, and the
// inter-op dependency edges. This is command output, not lifecycle:
// it fires once per `scampi plan` invocation and carries the data the
// CLI renders as the plan tree.
func PlanProduced(plan spec.Plan, actionDeps ActionDeps) event.PlanEvent {
	// Flatten all ops and assign global indices.
	var allOps []spec.Op
	opIndex := make(map[spec.Op]int)
	actionOpBase := make(map[int]int) // action index -> first op index
	for i, act := range plan.Unit.Actions {
		actionOpBase[i] = len(allOps)
		for _, op := range act.Ops() {
			opIndex[op] = len(allOps)
			allOps = append(allOps, op)
		}
	}

	// Build PlannedOps with dependency indices.
	plannedOps := make([]event.PlannedOp, len(allOps))
	for i, op := range allOps {
		var tmpl *spec.PlanTemplate
		if d, ok := op.(spec.OpDescriber); ok {
			if desc := d.OpDescription(); desc != nil {
				t := desc.PlanTemplate()
				tmpl = &t
			}
		}
		var deps []int
		for _, dep := range op.DependsOn() {
			deps = append(deps, opIndex[dep])
		}
		plannedOps[i] = event.PlannedOp{
			Index:     i,
			DisplayID: OpDisplayID(op),
			DependsOn: deps,
			Template:  tmpl, // nil = fallback
		}
	}

	// Re-slice ops back into PlannedActions.
	var detail event.PlanDetail
	for i, act := range plan.Unit.Actions {
		start := actionOpBase[i]
		end := start + len(act.Ops())
		var deps []int
		if actionDeps != nil && i < len(actionDeps) {
			deps = actionDeps[i]
		}
		detail.Actions = append(detail.Actions, event.PlannedAction{
			Index:     i,
			Desc:      act.Desc(),
			Kind:      act.Kind(),
			DependsOn: deps,
			Ops:       plannedOps[start:end],
		})
	}

	return event.PlanEvent{
		Time:       time.Now(),
		Kind:       event.PlanProduced,
		Detail:     &detail,
		Severity:   signal.Notice,
		Chattiness: event.Subtle,
	}
}

// Diagnostics
// -----------------------------------------------------------------------------

func RaiseEngineDiagnostic(cfgPath string, d Diagnostic) event.EngineDiagnostic {
	return event.EngineDiagnostic{
		Time:    time.Now(),
		CfgPath: cfgPath,
		Detail: event.DiagnosticDetail{
			Template: d.EventTemplate(),
		},
		Severity:   d.Severity(),
		Chattiness: event.Subtle,
	}
}

func RaisePlanDiagnostic(stepIdx int, stepKind, stepDesc string, d Diagnostic) event.PlanDiagnostic {
	return event.PlanDiagnostic{
		Time: time.Now(),
		Step: event.StepDetail{
			StepIndex: stepIdx,
			StepKind:  stepKind,
			StepDesc:  stepDesc,
		},
		Detail: event.DiagnosticDetail{
			Template: d.EventTemplate(),
		},
		Severity:   d.Severity(),
		Chattiness: event.Subtle,
	}
}

func RaiseActionDiagnostic(stepIdx int, stepKind, stepDesc string, d Diagnostic) event.ActionDiagnostic {
	return event.ActionDiagnostic{
		Time: time.Now(),
		Step: event.StepDetail{
			StepIndex: stepIdx,
			StepKind:  stepKind,
			StepDesc:  stepDesc,
		},
		Detail: event.DiagnosticDetail{
			Template: d.EventTemplate(),
		},
		Severity:   d.Severity(),
		Chattiness: event.Subtle,
	}
}

func RaiseOpDiagnostic(stepIdx int, stepKind, stepDesc, displayID string, d Diagnostic) event.OpDiagnostic {
	return event.OpDiagnostic{
		Time: time.Now(),
		Step: event.StepDetail{
			StepIndex: stepIdx,
			StepKind:  stepKind,
			StepDesc:  stepDesc,
		},
		DisplayID: displayID,
		Detail: event.DiagnosticDetail{
			Template: d.EventTemplate(),
		},
		Severity:   d.Severity(),
		Chattiness: event.Subtle,
	}
}

// New diagnostic surface - see doc/design/diagnostics.md.
// -----------------------------------------------------------------------------

// Raise constructs an event.Diagnostic from a Diagnostic value. Replaces
// the four scope-specific Raise*Diagnostic helpers above; scope is no
// longer an axis. Producers wanting to stamp a Cause should either set
// the field on the returned event or wrap the emitter via WithCause.
func Raise(d Diagnostic) event.Diagnostic {
	return event.Diagnostic{
		Time:     time.Now(),
		Severity: severityToV2(d.Severity()),
		Template: d.EventTemplate(),
	}
}

// severityToV2 maps the legacy six-level signal.Severity onto the
// three-level event.Severity. Debug/Info/Notice collapse to Info,
// Warning maps directly, Error/Fatal collapse to Error. The abort
// distinction that Fatal used to carry lives on Diagnostic.Impact and
// is unaffected by this mapping.
func severityToV2(s signal.Severity) event.Severity {
	switch s {
	case signal.Warning:
		return event.SeverityWarning
	case signal.Error, signal.Fatal:
		return event.SeverityError
	default:
		return event.SeverityInfo
	}
}
