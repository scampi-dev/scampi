// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"errors"
	"fmt"
	"runtime"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/signal"
	"godoit.dev/doit/spec"
)

type AbortError struct {
	Causes []error
}

func (AbortError) Error() string {
	return "execution aborted"
}

type CapabilityMismatchError struct {
	StepIndex    int
	StepKind     string
	RequiredCaps capability.Capability
	MissingCaps  capability.Capability
	ProvidedCaps capability.Capability
	Source       spec.SourceSpan
}

func (e CapabilityMismatchError) Error() string {
	return fmt.Sprintf(
		"step %q requires %s, but target only provides %s (missing: %s)",
		e.StepKind, e.RequiredCaps, e.ProvidedCaps, e.MissingCaps,
	)
}

func (e CapabilityMismatchError) EventTemplate() event.Template {
	return event.Template{
		ID:     "engine.CapabilityMismatch",
		Text:   `step "{{.StepKind}}" requires capabilities not provided by target`,
		Hint:   "use a different target or remove incompatible steps",
		Help:   "missing:  {{.MissingCaps}}\nrequired: {{.RequiredCaps}}\nprovided: {{.ProvidedCaps}}",
		Data:   e,
		Source: &e.Source,
	}
}

func (e CapabilityMismatchError) Severity() signal.Severity {
	return signal.Error
}

func (e CapabilityMismatchError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func panicIfNotAbortError(err error) error {
	var abort AbortError
	if errors.As(err, &abort) {
		return abort
	}
	// very cold codepath
	wrap := errs.BUG("Engine failed with non-signal error: %w", err)
	if pc, _, _, ok := runtime.Caller(1); ok {
		if details := runtime.FuncForPC(pc); details != nil {
			wrap = errs.BUG("%s failed with non-signal error: %w", details.Name(), err)
		}
	}
	panic(wrap)
}

// emitScopedDiagnostic extracts diagnostic(s) from err and passes each to emit.
// Returns the max impact and whether any diagnostic was emitted.
func emitScopedDiagnostic(err error, emit func(diagnostic.Diagnostic)) (diagnostic.Impact, bool) {
	if err == nil {
		return 0, false
	}

	var ds diagnostic.Diagnostics
	if errors.As(err, &ds) {
		impact := diagnostic.ImpactNone
		for _, d := range ds {
			emit(d)
			if d.Impact() > impact {
				impact = d.Impact()
			}
		}
		return impact, true
	}

	var d diagnostic.Diagnostic
	if !errors.As(err, &d) {
		return 0, false
	}

	emit(d)
	return d.Impact(), true
}

func emitEngineDiagnostic(em diagnostic.Emitter, cfgPath string, err error) (diagnostic.Impact, bool) {
	return emitScopedDiagnostic(err, func(d diagnostic.Diagnostic) {
		em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic(cfgPath, d))
	})
}

func emitPlanDiagnostic(
	em diagnostic.Emitter, stepIndex int, stepKind, stepDesc string, err error,
) (diagnostic.Impact, bool) {
	return emitScopedDiagnostic(err, func(d diagnostic.Diagnostic) {
		em.EmitPlanDiagnostic(diagnostic.RaisePlanDiagnostic(stepIndex, stepKind, stepDesc, d))
	})
}

func emitActionDiagnostic(
	em diagnostic.Emitter, stepIndex int, stepKind, stepDesc string, err error,
) (diagnostic.Impact, bool) {
	return emitScopedDiagnostic(err, func(d diagnostic.Diagnostic) {
		em.EmitActionDiagnostic(diagnostic.RaiseActionDiagnostic(stepIndex, stepKind, stepDesc, d))
	})
}

func emitOpDiagnostic(
	em diagnostic.Emitter, stepIndex int, stepKind, stepDesc, displayID string, err error,
) (diagnostic.Impact, bool) {
	return emitScopedDiagnostic(err, func(d diagnostic.Diagnostic) {
		em.EmitOpDiagnostic(diagnostic.RaiseOpDiagnostic(stepIndex, stepKind, stepDesc, displayID, d))
	})
}

// Index errors
// -----------------------------------------------------------------------------

type UnknownIndexKindError struct {
	Kind string
}

func (e UnknownIndexKindError) Error() string {
	return fmt.Sprintf("unknown step kind %q", e.Kind)
}

func (e UnknownIndexKindError) EventTemplate() event.Template {
	return event.Template{
		ID:   "index.UnknownKind",
		Text: `unknown step kind "{{.Kind}}"`,
		Hint: "use 'doit index' to list available step types",
		Data: e,
	}
}

func (UnknownIndexKindError) Severity() signal.Severity { return signal.Error }
func (UnknownIndexKindError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// Resolution errors
// -----------------------------------------------------------------------------

type UnknownDeployBlockError struct {
	Name string
}

func (e UnknownDeployBlockError) Error() string {
	return fmt.Sprintf("unknown deploy block %q", e.Name)
}

func (e UnknownDeployBlockError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.UnknownDeployBlock",
		Text: `unknown deploy block "{{.Name}}"`,
		Hint: "check that the deploy block name is spelled correctly",
		Data: e,
	}
}

func (UnknownDeployBlockError) Severity() signal.Severity { return signal.Error }
func (UnknownDeployBlockError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type NoDeployBlocksError struct{}

func (NoDeployBlocksError) Error() string {
	return "no deploy blocks defined"
}

func (NoDeployBlocksError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.NoDeployBlocks",
		Text: "no deploy blocks defined",
		Hint: "add at least one deploy block to the configuration",
	}
}

func (NoDeployBlocksError) Severity() signal.Severity { return signal.Error }
func (NoDeployBlocksError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type NoTargetsInDeployError struct {
	Deploy string
}

func (e NoTargetsInDeployError) Error() string {
	return fmt.Sprintf("deploy block %q has no targets", e.Deploy)
}

func (e NoTargetsInDeployError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.NoTargetsInDeploy",
		Text: `deploy block "{{.Deploy}}" has no targets`,
		Hint: "add at least one target to the deploy block's targets list",
		Data: e,
	}
}

func (NoTargetsInDeployError) Severity() signal.Severity { return signal.Error }
func (NoTargetsInDeployError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type UnknownTargetError struct {
	Name   string
	Deploy string
}

func (e UnknownTargetError) Error() string {
	return fmt.Sprintf("unknown target %q referenced in deploy block %q", e.Name, e.Deploy)
}

func (e UnknownTargetError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.UnknownTarget",
		Text: `unknown target "{{.Name}}" referenced in deploy block "{{.Deploy}}"`,
		Hint: "check that the target is defined in the targets map",
		Data: e,
	}
}

func (UnknownTargetError) Severity() signal.Severity { return signal.Error }
func (UnknownTargetError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

type TargetNotInDeployError struct {
	Target string
	Deploy string
}

func (e TargetNotInDeployError) Error() string {
	return fmt.Sprintf("target %q is not in deploy block %q's target list", e.Target, e.Deploy)
}

func (e TargetNotInDeployError) EventTemplate() event.Template {
	return event.Template{
		ID:   "config.TargetNotInDeploy",
		Text: `target "{{.Target}}" is not in deploy block "{{.Deploy}}"'s target list`,
		Hint: "add the target to the deploy block's targets list or select a different target",
		Data: e,
	}
}

func (TargetNotInDeployError) Severity() signal.Severity { return signal.Error }
func (TargetNotInDeployError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }
