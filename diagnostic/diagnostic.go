package diagnostic

import (
	"time"

	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/model"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type (
	Emitter interface {
		Emit(e event.Event)
	}
	Diagnostic interface {
		EventTemplate() event.Template
		Severity() signal.Severity
	}
	DiagnosticProvider interface {
		Diagnostics(subject event.Subject) []event.Event
	}
)

// Engine lifecycle
// ===============================================

func EngineStarted() event.Event {
	return event.Event{
		Time:       time.Now(),
		Kind:       event.EngineStarted,
		Scope:      event.ScopeEngine,
		Severity:   signal.Info,
		Chattiness: event.Subtle,
	}
}

func EngineFinished(rep model.ExecutionReport, dur time.Duration, err error) event.Event {
	var total, changed, failed int

	for _, ar := range rep.Actions {
		total += ar.Summary.Total
		changed += ar.Summary.Changed
		failed += ar.Summary.Failed
	}

	e := event.Event{
		Time:  time.Now(),
		Kind:  event.EngineFinished,
		Scope: event.ScopeEngine,
		Detail: event.EngineDetail{
			TotalCount:   total,
			ChangedCount: changed,
			FailedCount:  failed,
			Duration:     dur,
			Err:          err,
		},
	}

	switch {
	case err != nil:
		e.Severity = signal.Fatal
		e.Chattiness = event.Normal

	case failed > 0:
		e.Severity = signal.Error
		e.Chattiness = event.Normal

	case changed > 0:
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

func PlanStarted() event.Event {
	return event.Event{
		Time:       time.Now(),
		Kind:       event.PlanStarted,
		Scope:      event.ScopePlan,
		Severity:   signal.Info,
		Chattiness: event.Subtle,
	}
}

func PlanFinished(successfulUnits, failedUnits int, dur time.Duration) event.Event {
	e := event.Event{
		Time:  time.Now(),
		Kind:  event.PlanFinished,
		Scope: event.ScopePlan,
		Detail: event.PlanFinishedDetail{
			SuccessfulUnits: successfulUnits,
			FailedUnits:     failedUnits,
			Duration:        dur,
		},
	}

	switch {
	case failedUnits > 0:
		e.Severity = signal.Error
		e.Chattiness = event.Reserved

	case successfulUnits == 0:
		e.Severity = signal.Warning
		e.Chattiness = event.Normal

	default:
		e.Severity = signal.Info
		e.Chattiness = event.Subtle
	}

	return e
}

func UnitPlanned(
	index int,
	name string,
	kind string,
) event.Event {
	return event.Event{
		Time:  time.Now(),
		Kind:  event.UnitPlanned,
		Scope: event.ScopePlan,
		Subject: event.Subject{
			Index: index,
			Name:  name,
			Kind:  kind,
		},
		Severity:   signal.Debug,
		Chattiness: event.Chatty,
	}
}

func PlanProduced(plan spec.Plan) event.Event {
	// ------------------------------------------------------------
	// 1. Flatten all ops and assign GLOBAL indices
	// ------------------------------------------------------------
	var allOps []spec.Op
	opIndex := make(map[spec.Op]int)
	actionOpBase := make(map[int]int) // action index → first op index
	for i, act := range plan.Actions {
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
			Name:      op.Name(),
			DependsOn: deps,
			Template:  tmpl, // nil = fallback
		}
	}

	// ------------------------------------------------------------
	// 3. Re-slice ops back into PlannedActions
	// ------------------------------------------------------------
	var detail event.PlanDetail
	for i, act := range plan.Actions {
		start := actionOpBase[i]
		end := start + len(act.Ops())

		detail.Actions = append(detail.Actions, event.PlannedAction{
			Index: i,
			Name:  act.Name(),
			Kind:  act.Kind(),
			Ops:   plannedOps[start:end],
		})
	}

	return event.Event{
		Time:       time.Now(),
		Kind:       event.PlanProduced,
		Scope:      event.ScopePlan,
		Detail:     detail,
		Severity:   signal.Notice,
		Chattiness: event.Subtle,
	}
}

// Action lifecycle
// ===============================================

func ActionStarted(name string) event.Event {
	return event.Event{
		Time:  time.Now(),
		Kind:  event.ActionStarted,
		Scope: event.ScopeAction,
		Subject: event.Subject{
			Action: name,
		},
		Severity:   signal.Notice,
		Chattiness: event.Normal,
	}
}

func ActionFinished(name string, summary model.ActionSummary, dur time.Duration, err error) event.Event {
	e := event.Event{
		Time:  time.Now(),
		Kind:  event.ActionFinished,
		Scope: event.ScopeAction,
		Subject: event.Subject{
			Action: name,
		},
		Detail: event.ActionDetail{
			Summary:  summary,
			Duration: dur,
			Err:      err,
		},
	}

	s := summary
	switch {

	case s.Failed > 0 || s.Aborted > 0 || err != nil:
		e.Severity = signal.Error
		e.Chattiness = event.Normal

	case s.Changed > 0:
		e.Severity = signal.Notice
		e.Chattiness = event.Normal

	default:
		e.Severity = signal.Info
		e.Chattiness = event.Reserved
	}

	return e
}

// Op lifecycle
// ===============================================

func OpCheckStarted(action, op string) event.Event {
	return event.Event{
		Time:  time.Now(),
		Kind:  event.OpCheckStarted,
		Scope: event.ScopeOp,
		Subject: event.Subject{
			Action: action,
			Op:     op,
		},
		Severity:   signal.Debug,
		Chattiness: event.Chatty,
	}
}

func OpChecked(action, op string, res spec.CheckResult, err error) event.Event {
	e := event.Event{
		Time:  time.Now(),
		Kind:  event.OpChecked,
		Scope: event.ScopeOp,
		Subject: event.Subject{
			Action: action,
			Op:     op,
		},
		Detail: event.OpCheckDetail{
			Result: res,
			Err:    err,
		},
	}

	switch res {
	case spec.CheckSatisfied:
		e.Severity = signal.Info
		e.Chattiness = event.Subtle

	case spec.CheckUnsatisfied:
		e.Severity = signal.Notice
		e.Chattiness = event.Normal

	case spec.CheckUnknown:
		e.Severity = signal.Warning
		e.Chattiness = event.Reserved
	}

	return e
}

func OpExecuteStarted(action, op string) event.Event {
	return event.Event{
		Time:  time.Now(),
		Kind:  event.OpExecuteStarted,
		Scope: event.ScopeOp,
		Subject: event.Subject{
			Action: action,
			Op:     op,
		},
		Severity:   signal.Info,
		Chattiness: event.Chatty,
	}
}

func OpExecuted(action, op string, changed bool, dur time.Duration, err error) event.Event {
	e := event.Event{
		Time:  time.Now(),
		Kind:  event.OpExecuted,
		Scope: event.ScopeOp,
		Subject: event.Subject{
			Action: action,
			Op:     op,
		},
		Detail: event.OpExecuteDetail{
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

// Diagnostics
// ===============================================

func DiagnosticRaised(subject event.Subject, d Diagnostic) event.Event {
	var scope event.Scope
	switch {
	case subject.Op != "":
		scope = event.ScopeOp
	case subject.Action != "":
		scope = event.ScopeAction
	case subject.CfgPath != "":
		scope = event.ScopeEngine
	default:
		scope = event.ScopePlan
	}

	return event.Event{
		Time:    time.Now(),
		Kind:    event.DiagnosticRaised,
		Scope:   scope,
		Subject: subject,
		Detail: event.DiagnosticDetail{
			Template: d.EventTemplate(),
		},
		Severity:   d.Severity(),
		Chattiness: event.Subtle,
	}
}
