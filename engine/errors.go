// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

type AbortError struct {
	Causes []error
}

func (AbortError) Error() string {
	return "execution aborted"
}

// LoadConfigError is the fallback diagnostic emitted when the linker
// returns a non-diagnostic error (e.g. raw file-read failure). It
// guarantees the user sees *something* in the render pipeline rather
// than a silent abort.
type LoadConfigError struct {
	diagnostic.FatalError
	Cause  error
	Source spec.SourceSpan
}

func (e *LoadConfigError) Error() string {
	if e.Cause == nil {
		return "failed to load config"
	}
	return "failed to load config: " + e.Cause.Error()
}

func (e *LoadConfigError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeLoadConfigError,
		Text: "{{.Message}}",
		Hint: "check the config file path and any imported modules",
		Data: loadConfigErrorData{
			Message: e.Error(),
		},
		Source: &e.Source,
	}
}

type loadConfigErrorData struct {
	Message string
}

// CancelledError is returned when execution is interrupted by a signal
// (e.g. Ctrl+C). This is normal control flow, not a bug.
type CancelledError struct{}

func (CancelledError) Error() string {
	return "interrupted"
}

type CapabilityMismatchError struct {
	diagnostic.FatalError
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
		ID:     CodeCapabilityMismatch,
		Text:   `step "{{.StepKind}}" requires capabilities not provided by target`,
		Hint:   "use a different target or remove incompatible steps",
		Help:   "missing:  {{.MissingCaps}}\nrequired: {{.RequiredCaps}}\nprovided: {{.ProvidedCaps}}",
		Data:   e,
		Source: &e.Source,
	}
}

func panicIfNotAbortError(err error) error {
	var abort AbortError
	if errors.As(err, &abort) {
		return abort
	}
	if errors.Is(err, context.Canceled) {
		return CancelledError{}
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
	em diagnostic.Emitter,
	stepIndex int,
	stepKind, stepDesc string,
	err error,
) (diagnostic.Impact, bool) {
	return emitScopedDiagnostic(err, func(d diagnostic.Diagnostic) {
		em.EmitPlanDiagnostic(diagnostic.RaisePlanDiagnostic(stepIndex, stepKind, stepDesc, d))
	})
}

func emitActionDiagnostic(
	em diagnostic.Emitter,
	stepIndex int,
	stepKind, stepDesc string,
	err error,
) (diagnostic.Impact, bool) {
	return emitScopedDiagnostic(err, func(d diagnostic.Diagnostic) {
		em.EmitActionDiagnostic(diagnostic.RaiseActionDiagnostic(stepIndex, stepKind, stepDesc, d))
	})
}

func emitOpDiagnostic(
	em diagnostic.Emitter,
	stepIndex int,
	stepKind, stepDesc, displayID string,
	err error,
) (diagnostic.Impact, bool) {
	return emitScopedDiagnostic(err, func(d diagnostic.Diagnostic) {
		em.EmitOpDiagnostic(diagnostic.RaiseOpDiagnostic(stepIndex, stepKind, stepDesc, displayID, d))
	})
}

// Index errors
// -----------------------------------------------------------------------------

type UnknownIndexKindError struct {
	diagnostic.FatalError
	Kind string
}

func (e UnknownIndexKindError) Error() string {
	return fmt.Sprintf("unknown step kind %q", e.Kind)
}

func (e UnknownIndexKindError) EventTemplate() event.Template {
	return event.Template{
		ID:   CodeUnknownIndexKind,
		Text: `unknown step kind "{{.Kind}}"`,
		Hint: "use 'scampi index' to list available step types",
		Data: e,
	}
}

// Resolution errors
// -----------------------------------------------------------------------------

type UnknownDeployBlockError struct {
	diagnostic.FatalError
	Name   string
	Source spec.SourceSpan
}

func (e UnknownDeployBlockError) Error() string {
	return fmt.Sprintf("unknown deploy block %q", e.Name)
}

func (e UnknownDeployBlockError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeUnknownDeployBlock,
		Text:   `unknown deploy block "{{.Name}}"`,
		Hint:   "check that the deploy block name is spelled correctly",
		Data:   e,
		Source: &e.Source,
	}
}

type NoDeployBlocksError struct {
	diagnostic.FatalError
	Source spec.SourceSpan
}

func (NoDeployBlocksError) Error() string {
	return "no deploy blocks defined"
}

func (e NoDeployBlocksError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeNoDeployBlocks,
		Text:   "no deploy blocks defined",
		Hint:   "add at least one deploy block to the configuration",
		Data:   e,
		Source: &e.Source,
	}
}

type NoTargetsInDeployError struct {
	diagnostic.FatalError
	Deploy string
	Source spec.SourceSpan
}

func (e NoTargetsInDeployError) Error() string {
	return fmt.Sprintf("deploy block %q has no targets", e.Deploy)
}

func (e NoTargetsInDeployError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeNoTargetsInDeploy,
		Text:   `deploy block "{{.Deploy}}" has no targets`,
		Hint:   "add at least one target to the deploy block's targets list",
		Data:   e,
		Source: &e.Source,
	}
}

type UnknownTargetError struct {
	diagnostic.FatalError
	Name   string
	Deploy string
	Source spec.SourceSpan
}

func (e UnknownTargetError) Error() string {
	return fmt.Sprintf("unknown target %q referenced in deploy block %q", e.Name, e.Deploy)
}

func (e UnknownTargetError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeUnknownTarget,
		Text:   `unknown target "{{.Name}}" referenced in deploy block "{{.Deploy}}"`,
		Hint:   "check that the target is defined in the targets map",
		Data:   e,
		Source: &e.Source,
	}
}

type TargetNotInDeployError struct {
	diagnostic.FatalError
	Target string
	Deploy string
	Source spec.SourceSpan
}

func (e TargetNotInDeployError) Error() string {
	return fmt.Sprintf("target %q is not in deploy block %q's target list", e.Target, e.Deploy)
}

func (e TargetNotInDeployError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeTargetNotInDeploy,
		Text:   `target "{{.Target}}" is not in deploy block "{{.Deploy}}"'s target list`,
		Hint:   "add the target to the deploy block's targets list or select a different target",
		Data:   e,
		Source: &e.Source,
	}
}

// Hook errors
// -----------------------------------------------------------------------------

type UnknownHookError struct {
	diagnostic.FatalError
	HookID   string
	StepKind string
	StepDesc string
	Source   spec.SourceSpan
}

func (e UnknownHookError) Error() string {
	return fmt.Sprintf("on_change references unknown hook %q", e.HookID)
}

func (e UnknownHookError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeUnknownHook,
		Text:   `on_change references unknown hook "{{.HookID}}"`,
		Hint:   `add hooks = {"{{.HookID}}": service(name="...", state="restarted")} to the deploy block`,
		Data:   e,
		Source: &e.Source,
	}
}

type HookCycleError struct {
	diagnostic.FatalError
	Chain  []string
	Source spec.SourceSpan
}

func (e HookCycleError) Error() string {
	return fmt.Sprintf("hook cycle detected: %s", strings.Join(e.Chain, " -> "))
}

func (e HookCycleError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeHookCycle,
		Text:   "hook cycle detected",
		Hint:   `{{join " -> " .Chain}}`,
		Data:   e,
		Source: &e.Source,
	}
}
