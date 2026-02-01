//go:generate stringer -type=Impact
package diagnostic

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/model"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

// OpDisplayID derives a display identifier for an op.
// Uses OpDescriber template ID if available, otherwise falls back to type name.
func OpDisplayID(op spec.Op) string {
	if d, ok := op.(spec.OpDescriber); ok {
		if desc := d.OpDescription(); desc != nil {
			if id := desc.PlanTemplate().ID; id != "" {
				return id
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

type (
	Emitter interface {
		EmitEngineLifecycle(e event.EngineEvent)
		EmitPlanLifecycle(e event.PlanEvent)
		EmitActionLifecycle(e event.ActionEvent)
		EmitOpLifecycle(e event.OpEvent)

		EmitIndexAll(e event.IndexAllEvent)
		EmitIndexStep(e event.IndexStepEvent)

		EmitEngineDiagnostic(e event.EngineDiagnostic)
		EmitPlanDiagnostic(e event.PlanDiagnostic)
		EmitActionDiagnostic(e event.ActionDiagnostic)
		EmitOpDiagnostic(e event.OpDiagnostic)
	}
	Diagnostic interface {
		EventTemplate() event.Template
		Severity() signal.Severity
		Impact() Impact
	}
	Diagnostics     []Diagnostic
	MultiDiagnostic interface {
		Diagnostics() []Diagnostic
	}
)

func (d Diagnostics) Diagnostics() []Diagnostic { return d }
func (d Diagnostics) Error() string             { return fmt.Sprintf("%#v", d) }

// Engine lifecycle
// ===============================================

func EngineStarted() event.EngineEvent {
	return event.EngineEvent{
		Time:       time.Now(),
		Kind:       event.EngineStarted,
		Severity:   signal.Info,
		Chattiness: event.Subtle,
	}
}

func EngineFinished(rep model.ExecutionReport, dur time.Duration, err error, checkOnly bool) event.EngineEvent {
	e := event.EngineEvent{
		Time: time.Now(),
		Kind: event.EngineFinished,
		Detail: &event.EngineFinishedDetail{
			CheckOnly: checkOnly,
			Duration:  dur,
			Err:       err,
		},
	}

	for _, ar := range rep.Actions {
		e.Detail.TotalCount += ar.Summary.Total
		e.Detail.ChangedCount += ar.Summary.Changed
		e.Detail.WouldChangeCount += ar.Summary.WouldChange
		e.Detail.FailedCount += ar.Summary.Failed
	}

	switch {
	case err != nil:
		e.Severity = signal.Fatal
		e.Chattiness = event.Normal

	case e.Detail.FailedCount > 0:
		e.Severity = signal.Error
		e.Chattiness = event.Normal

	case e.Detail.ChangedCount > 0 || e.Detail.WouldChangeCount > 0:
		e.Severity = signal.Notice
		e.Chattiness = event.Subtle

	default:
		e.Severity = signal.Info
		e.Chattiness = event.Subtle
	}

	return e
}

// Plan lifecycle
// ===============================================

func PlanStarted(unitID spec.UnitID) event.PlanEvent {
	return event.PlanEvent{
		Time: time.Now(),
		Kind: event.PlanStarted,
		StartedDetail: &event.PlanStartedDetail{
			UnitID: string(unitID),
		},
		Severity:   signal.Info,
		Chattiness: event.Subtle,
	}
}

func PlanFinished(unitID spec.UnitID, successfulSteps, failedSteps int, dur time.Duration) event.PlanEvent {
	e := event.PlanEvent{
		Time: time.Now(),
		Kind: event.PlanFinished,
		FinishedDetail: &event.PlanFinishedDetail{
			UnitID:          string(unitID),
			SuccessfulSteps: successfulSteps,
			FailedSteps:     failedSteps,
			Duration:        dur,
		},
	}

	switch {
	case failedSteps > 0:
		e.Severity = signal.Error
		e.Chattiness = event.Reserved

	case successfulSteps == 0:
		e.Severity = signal.Warning
		e.Chattiness = event.Normal

	default:
		e.Severity = signal.Info
		e.Chattiness = event.Subtle
	}

	return e
}

func StepPlanned(index int, desc string, kind string) event.PlanEvent {
	return event.PlanEvent{
		Time: time.Now(),
		Kind: event.StepPlanned,
		Step: event.StepDetail{
			StepIndex: index,
			StepDesc:  desc,
			StepKind:  kind,
		},
		Severity:   signal.Debug,
		Chattiness: event.Chatty,
	}
}

func PlanProduced(plan spec.Plan) event.PlanEvent {
	// ------------------------------------------------------------
	// 1. Flatten all ops and assign GLOBAL indices
	// ------------------------------------------------------------
	var allOps []spec.Op
	opIndex := make(map[spec.Op]int)
	actionOpBase := make(map[int]int) // action index → first op index
	for i, act := range plan.Unit.Actions {
		actionOpBase[i] = len(allOps)
		for _, op := range act.Ops() {
			opIndex[op] = len(allOps)
			allOps = append(allOps, op)
		}
	}

	// ------------------------------------------------------------
	// 2. Build PlannedOps with dependency indices
	// ------------------------------------------------------------
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

	// ------------------------------------------------------------
	// 3. Re-slice ops back into PlannedActions
	// ------------------------------------------------------------
	var detail event.PlanDetail
	for i, act := range plan.Unit.Actions {
		start := actionOpBase[i]
		end := start + len(act.Ops())

		detail.Actions = append(detail.Actions, event.PlannedAction{
			Index: i,
			Desc:  act.Desc(),
			Kind:  act.Kind(),
			Ops:   plannedOps[start:end],
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

// Action lifecycle
// ===============================================

func ActionStarted(idx int, kind, desc string) event.ActionEvent {
	return event.ActionEvent{
		Time: time.Now(),
		Kind: event.ActionStarted,
		Step: event.StepDetail{
			StepIndex: idx,
			StepKind:  kind,
			StepDesc:  desc,
		},
		Severity:   signal.Notice,
		Chattiness: event.Normal,
	}
}

func ActionFinished(
	idx int,
	kind,
	desc string,
	summary model.ActionSummary,
	dur time.Duration,
	err error,
) event.ActionEvent {
	e := event.ActionEvent{
		Time: time.Now(),
		Kind: event.ActionFinished,
		Step: event.StepDetail{
			StepIndex: idx,
			StepKind:  kind,
			StepDesc:  desc,
		},
		Detail: &event.ActionDetail{
			Summary:  summary,
			Duration: dur,
			Err:      err,
		},
	}

	s := summary
	switch {

	case s.Failed > 0 || s.Aborted > 0 || err != nil:
		e.Severity = signal.Error
		e.Chattiness = event.Subtle

	case s.Changed > 0 || s.WouldChange > 0:
		e.Severity = signal.Notice
		e.Chattiness = event.Subtle

	default:
		e.Severity = signal.Info
		e.Chattiness = event.Normal
	}

	return e
}

// Op lifecycle
// ===============================================

func OpCheckStarted(stepIdx int, stepKind, stepDesc, displayID string) event.OpEvent {
	return event.OpEvent{
		Time: time.Now(),
		Kind: event.OpCheckStarted,
		Step: event.StepDetail{
			StepIndex: stepIdx,
			StepKind:  stepKind,
			StepDesc:  stepDesc,
		},
		DisplayID:  displayID,
		Severity:   signal.Debug,
		Chattiness: event.Chatty,
	}
}

func OpChecked(
	stepIdx int,
	stepKind,
	stepDesc,
	displayID string,
	res spec.CheckResult,
	err error,
	checkOnly bool,
) event.OpEvent {
	e := event.OpEvent{
		Time: time.Now(),
		Kind: event.OpChecked,
		Step: event.StepDetail{
			StepIndex: stepIdx,
			StepKind:  stepKind,
			StepDesc:  stepDesc,
		},
		DisplayID: displayID,
		CheckDetail: &event.OpCheckDetail{
			Result: res,
			Err:    err,
		},
	}

	switch res {
	case spec.CheckSatisfied:
		e.Severity = signal.Info
		e.Chattiness = event.Chatty

	case spec.CheckUnsatisfied:
		e.Severity = signal.Notice
		// In check-only mode, show "needs change" at -v level
		// In apply mode, show at -vv level (execution results are more important)
		if checkOnly {
			e.Chattiness = event.Normal
		} else {
			e.Chattiness = event.Chatty
		}

	case spec.CheckUnknown:
		e.Severity = signal.Warning
		e.Chattiness = event.Reserved
	}

	return e
}

func OpExecuteStarted(stepIdx int, stepKind, stepDesc, displayID string) event.OpEvent {
	return event.OpEvent{
		Time: time.Now(),
		Kind: event.OpExecuteStarted,
		Step: event.StepDetail{
			StepIndex: stepIdx,
			StepKind:  stepKind,
			StepDesc:  stepDesc,
		},
		DisplayID:  displayID,
		Severity:   signal.Info,
		Chattiness: event.Chatty,
	}
}

func OpExecuted(
	stepIdx int,
	stepKind string,
	stepDesc string,
	displayID string,
	changed bool,
	dur time.Duration,
	err error,
) event.OpEvent {
	e := event.OpEvent{
		Time: time.Now(),
		Kind: event.OpExecuted,
		Step: event.StepDetail{
			StepIndex: stepIdx,
			StepKind:  stepKind,
			StepDesc:  stepDesc,
		},
		DisplayID: displayID,
		ExecuteDetail: &event.OpExecuteDetail{
			Changed:  changed,
			Duration: dur,
			Err:      err,
		},
	}

	switch {
	case err != nil:
		e.Severity = signal.Error
		e.Chattiness = event.Normal

	case changed:
		e.Severity = signal.Notice
		e.Chattiness = event.Normal

	default:
		e.Severity = signal.Info
		e.Chattiness = event.Reserved
	}

	return e
}

func IndexAllProduced(docs []spec.StepDoc) event.IndexAllEvent {
	steps := make([]event.StepIndexDetail, len(docs))
	for i, doc := range docs {
		steps[i] = event.StepIndexDetail{
			Kind: doc.Kind,
			Desc: doc.Summary,
		}
	}

	slices.SortStableFunc(steps, func(a event.StepIndexDetail, b event.StepIndexDetail) int {
		return strings.Compare(a.Kind, b.Kind)
	})

	return event.IndexAllEvent{
		Time:       time.Now(),
		Steps:      steps,
		Severity:   signal.Notice,
		Chattiness: event.Subtle,
	}
}

func IndexStepProduced(doc spec.StepDoc) event.IndexStepEvent {
	return event.IndexStepEvent{
		Time:       time.Now(),
		Doc:        doc,
		Severity:   signal.Notice,
		Chattiness: event.Subtle,
	}
}

// Diagnostics
// ===============================================

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
