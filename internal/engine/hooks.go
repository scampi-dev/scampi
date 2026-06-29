// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/result"
	"scampi.dev/scampi/internal/spec"
)

// executeHooks runs notified hooks after all deploy steps complete.
// It collects which hooks were notified by steps that changed, then executes
// them in notification order. Hook chaining is supported: if a hook has
// on_change and it reports changes, those hooks are added to the queue.
// Each hook fires at most once per run.
func (e *Engine) executeHooks(
	ctx diagnostic.Ctx,
	stepReport result.Execution,
	hp *hookPlan,
	checkOnly bool,
	promisedPaths map[spec.Resource]bool,
) (result.Execution, error) {
	if hp == nil || len(hp.steps) == 0 {
		return result.Execution{}, nil
	}

	// Collect notified hooks from step results, preserving notification order.
	var queue []string
	notified := map[string]bool{}
	triggerBy := map[string]string{} // hook ID → desc of step that triggered it

	for i, ar := range stepReport.Steps {
		onChange, ok := hp.onChange[i]
		if !ok {
			continue
		}

		changed := stepChanged(ar, checkOnly)
		if !changed {
			continue
		}

		for _, hookID := range onChange {
			if !notified[hookID] {
				notified[hookID] = true
				triggerBy[hookID] = ar.Step.Desc()
				queue = append(queue, hookID)
			}
		}
	}

	// Execute notified hooks. Process queue — new entries may be appended
	// by hook chaining.
	var hookReports []result.StepReport
	executed := map[string]bool{}

	for i := 0; i < len(queue); i++ {
		hookID := queue[i]
		if executed[hookID] {
			continue
		}
		executed[hookID] = true

		steps, ok := hp.steps[hookID]
		if !ok {
			continue
		}

		anyChanged := false
		var hookErr error

		for _, act := range steps {
			hookIdx := len(stepReport.Steps) + len(hookReports)

			var ar result.StepReport
			var err error
			if checkOnly {
				ar, err = e.runCheckStep(ctx, hookIdx, act, promisedPaths, hookID)
			} else {
				ar, err = e.runStep(ctx, hookIdx, act, hookID)
			}

			hookReports = append(hookReports, ar)

			if stepChanged(ar, checkOnly) {
				anyChanged = true
			}

			if err != nil {
				hookErr = err
				break
			}
		}

		if hookErr != nil {
			return result.Execution{
				Steps: hookReports,
				Err:   hookErr,
			}, hookErr
		}

		// Handle chaining: if any step in this hook changed, notify on_change targets
		if anyChanged {
			if steps, ok := e.cfg.Hooks[hookID]; ok {
				for _, step := range steps {
					for _, nextID := range step.OnChange {
						if !notified[nextID] {
							notified[nextID] = true
							triggerBy[nextID] = "hook:" + hookID
							queue = append(queue, nextID)
						}
					}
				}
			}
		}
	}

	return result.Execution{Steps: hookReports}, nil
}

// stepChanged returns true if a step report indicates something changed
// (or would change in check mode).
func stepChanged(ar result.StepReport, checkOnly bool) bool {
	if checkOnly {
		return ar.Summary.WouldChange > 0
	}
	return ar.Summary.Changed > 0
}
