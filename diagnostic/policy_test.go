// SPDX-License-Identifier: GPL-3.0-only

package diagnostic

import (
	"sync"
	"testing"

	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

func TestPolicyEmitter_DedupEngineDiagnostic(t *testing.T) {
	rec := &recordingDisplayer{}
	em := NewEmitter(Policy{DedupDiagnostics: true}, rec)

	ev := makeEngineDiag("E001", "x out of range", &spec.SourceSpan{StartLine: 5})
	em.EmitEngineDiagnostic(ev)
	em.EmitEngineDiagnostic(ev)
	em.EmitEngineDiagnostic(ev)

	if got := len(rec.engine); got != 1 {
		t.Errorf("expected 1 engine diagnostic, got %d", got)
	}
}

func TestPolicyEmitter_DedupPlanDiagnostic(t *testing.T) {
	rec := &recordingDisplayer{}
	em := NewEmitter(Policy{DedupDiagnostics: true}, rec)

	ev := event.PlanDiagnostic{
		Detail:   event.DiagnosticDetail{Template: makeTemplate("P001", "step 0 broken")},
		Severity: signal.Error,
	}
	em.EmitPlanDiagnostic(ev)
	em.EmitPlanDiagnostic(ev)

	if got := len(rec.plan); got != 1 {
		t.Errorf("expected 1 plan diagnostic, got %d", got)
	}
}

func TestPolicyEmitter_DedupAcrossSeparateAllocations(t *testing.T) {
	rec := &recordingDisplayer{}
	em := NewEmitter(Policy{DedupDiagnostics: true}, rec)

	// Same content but distinct *spec.SourceSpan pointers — must
	// still dedup, since reflect.DeepEqual chases the pointer.
	a := makeEngineDiag("E002", "same content", &spec.SourceSpan{StartLine: 10})
	b := makeEngineDiag("E002", "same content", &spec.SourceSpan{StartLine: 10})

	em.EmitEngineDiagnostic(a)
	em.EmitEngineDiagnostic(b)

	if got := len(rec.engine); got != 1 {
		t.Errorf("expected 1 engine diagnostic with separate-pointer spans, got %d", got)
	}
}

func TestPolicyEmitter_DistinctTemplatesNotDeduped(t *testing.T) {
	rec := &recordingDisplayer{}
	em := NewEmitter(Policy{DedupDiagnostics: true}, rec)

	// Same span, same template ID, but different Data — these
	// render differently and must both reach the displayer.
	src := &spec.SourceSpan{StartLine: 12}
	a := event.EngineDiagnostic{
		Detail: event.DiagnosticDetail{Template: event.Template{
			ID:     "E003",
			Text:   "violation",
			Source: src,
			Data:   "first attribute",
		}},
		Severity: signal.Error,
	}
	b := event.EngineDiagnostic{
		Detail: event.DiagnosticDetail{Template: event.Template{
			ID:     "E003",
			Text:   "violation",
			Source: src,
			Data:   "second attribute",
		}},
		Severity: signal.Error,
	}

	em.EmitEngineDiagnostic(a)
	em.EmitEngineDiagnostic(b)

	if got := len(rec.engine); got != 2 {
		t.Errorf("expected 2 distinct engine diagnostics, got %d", got)
	}
}

func TestPolicyEmitter_DedupDisabled(t *testing.T) {
	rec := &recordingDisplayer{}
	em := NewEmitter(Policy{DedupDiagnostics: false}, rec)

	ev := makeEngineDiag("E004", "noisy", nil)
	em.EmitEngineDiagnostic(ev)
	em.EmitEngineDiagnostic(ev)
	em.EmitEngineDiagnostic(ev)

	if got := len(rec.engine); got != 3 {
		t.Errorf("expected 3 engine diagnostics with dedup off, got %d", got)
	}
}

func TestPolicyEmitter_LifecycleNotDeduped(t *testing.T) {
	rec := &recordingDisplayer{}
	em := NewEmitter(Policy{DedupDiagnostics: true}, rec)

	ev := event.EngineEvent{Severity: signal.Info}
	em.EmitEngineLifecycle(ev)
	em.EmitEngineLifecycle(ev)
	em.EmitEngineLifecycle(ev)

	if got := len(rec.engineLifecycle); got != 3 {
		t.Errorf("expected 3 lifecycle events even with dedup on, got %d", got)
	}
}

// TestPolicyEmitter_ConcurrentDedup_Race is the regression test for #329.
// Engine/plan/action/op diagnostics are emitted from concurrent goroutines
// (op pool, plan workers); the dedup slice must tolerate that. Run with
// `-race` — without the mutex this test trips the detector on the slice
// append inside shouldEmit.
func TestPolicyEmitter_ConcurrentDedup_Race(t *testing.T) {
	rec := &concurrentDisplayer{}
	em := NewEmitter(Policy{DedupDiagnostics: true}, rec)

	const (
		workers     = 16
		perWorker   = 100
		templateIDs = 8
	)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := range workers {
		go func() {
			defer wg.Done()
			for i := range perWorker {
				id := errs.Code("R") + errs.Code(rune('0'+byte((w*perWorker+i)%templateIDs)))
				em.EmitEngineDiagnostic(event.EngineDiagnostic{
					Detail:   event.DiagnosticDetail{Template: event.Template{ID: id, Text: "x"}},
					Severity: signal.Error,
				})
			}
		}()
	}
	wg.Wait()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if got := len(rec.engine); got == 0 || got > templateIDs {
		t.Errorf("expected 1..%d emissions after dedup, got %d", templateIDs, got)
	}
}

func TestPolicyEmitter_PerMethodIsolation(t *testing.T) {
	rec := &recordingDisplayer{}
	em := NewEmitter(Policy{DedupDiagnostics: true}, rec)

	tmpl := makeTemplate("X001", "shared template")

	em.EmitEngineDiagnostic(event.EngineDiagnostic{
		Detail:   event.DiagnosticDetail{Template: tmpl},
		Severity: signal.Error,
	})
	em.EmitPlanDiagnostic(event.PlanDiagnostic{
		Detail:   event.DiagnosticDetail{Template: tmpl},
		Severity: signal.Error,
	})

	// Different routing surfaces — both should reach the displayer.
	if got := len(rec.engine); got != 1 {
		t.Errorf("expected 1 engine diagnostic, got %d", got)
	}
	if got := len(rec.plan); got != 1 {
		t.Errorf("expected 1 plan diagnostic, got %d", got)
	}
}

// helpers

func makeEngineDiag(id, text string, src *spec.SourceSpan) event.EngineDiagnostic {
	return event.EngineDiagnostic{
		Detail: event.DiagnosticDetail{Template: event.Template{
			ID:     errs.Code(id),
			Text:   text,
			Source: src,
		}},
		Severity: signal.Error,
	}
}

func makeTemplate(id, text string) event.Template {
	return event.Template{ID: errs.Code(id), Text: text}
}

// recordingDisplayer is a minimal Displayer that captures
// each emission for later inspection.
type recordingDisplayer struct {
	engineLifecycle []event.EngineEvent
	planLifecycle   []event.PlanEvent
	actionLifecycle []event.ActionEvent
	opLifecycle     []event.OpEvent
	indexAll        []event.IndexAllEvent
	indexStep       []event.IndexStepEvent
	inspect         []event.InspectEvent

	engine []event.EngineDiagnostic
	plan   []event.PlanDiagnostic
	action []event.ActionDiagnostic
	op     []event.OpDiagnostic
}

func (r *recordingDisplayer) EmitEngineLifecycle(e event.EngineEvent) {
	r.engineLifecycle = append(r.engineLifecycle, e)
}
func (r *recordingDisplayer) EmitPlanLifecycle(e event.PlanEvent) {
	r.planLifecycle = append(r.planLifecycle, e)
}
func (r *recordingDisplayer) EmitActionLifecycle(e event.ActionEvent) {
	r.actionLifecycle = append(r.actionLifecycle, e)
}
func (r *recordingDisplayer) EmitOpLifecycle(e event.OpEvent) {
	r.opLifecycle = append(r.opLifecycle, e)
}
func (r *recordingDisplayer) EmitIndexAll(e event.IndexAllEvent) { r.indexAll = append(r.indexAll, e) }
func (r *recordingDisplayer) EmitIndexStep(e event.IndexStepEvent) {
	r.indexStep = append(r.indexStep, e)
}
func (r *recordingDisplayer) EmitInspect(e event.InspectEvent) { r.inspect = append(r.inspect, e) }
func (r *recordingDisplayer) EmitGraph(_ event.GraphEvent)     {}
func (r *recordingDisplayer) EmitLegend()                      {}
func (r *recordingDisplayer) EmitEngineDiagnostic(e event.EngineDiagnostic) {
	r.engine = append(r.engine, e)
}
func (r *recordingDisplayer) EmitPlanDiagnostic(e event.PlanDiagnostic) {
	r.plan = append(r.plan, e)
}
func (r *recordingDisplayer) EmitActionDiagnostic(e event.ActionDiagnostic) {
	r.action = append(r.action, e)
}
func (r *recordingDisplayer) EmitOpDiagnostic(e event.OpDiagnostic) {
	r.op = append(r.op, e)
}
func (r *recordingDisplayer) Interrupt() {}
func (r *recordingDisplayer) Close()     {}

// concurrentDisplayer is the recording variant used by the race test.
// Its only job is to keep its own slice append off the contended path —
// what we're testing is policyEmitter's seen-slice safety, not the
// displayer's.
type concurrentDisplayer struct {
	mu     sync.Mutex
	engine []event.EngineDiagnostic
}

func (c *concurrentDisplayer) EmitEngineLifecycle(event.EngineEvent) {}
func (c *concurrentDisplayer) EmitPlanLifecycle(event.PlanEvent)     {}
func (c *concurrentDisplayer) EmitActionLifecycle(event.ActionEvent) {}
func (c *concurrentDisplayer) EmitOpLifecycle(event.OpEvent)         {}
func (c *concurrentDisplayer) EmitIndexAll(event.IndexAllEvent)      {}
func (c *concurrentDisplayer) EmitIndexStep(event.IndexStepEvent)    {}
func (c *concurrentDisplayer) EmitInspect(event.InspectEvent)        {}
func (c *concurrentDisplayer) EmitGraph(event.GraphEvent)            {}
func (c *concurrentDisplayer) EmitLegend()                           {}
func (c *concurrentDisplayer) EmitEngineDiagnostic(e event.EngineDiagnostic) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.engine = append(c.engine, e)
}
func (c *concurrentDisplayer) EmitPlanDiagnostic(event.PlanDiagnostic)     {}
func (c *concurrentDisplayer) EmitActionDiagnostic(event.ActionDiagnostic) {}
func (c *concurrentDisplayer) EmitOpDiagnostic(event.OpDiagnostic)         {}
func (c *concurrentDisplayer) Interrupt()                                  {}
func (c *concurrentDisplayer) Close()                                      {}
