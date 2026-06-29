// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"errors"
	"testing"

	"scampi.dev/scampi/internal/spec"
)

type fakeStepType struct{ kind string }

func (f fakeStepType) Kind() string                                { return f.kind }
func (f fakeStepType) NewConfig() any                              { return nil }
func (f fakeStepType) Plan(spec.StepInstance) (spec.Action, error) { return nil, nil }

func mkStep(kind, file string, line int) spec.StepInstance {
	return spec.StepInstance{
		Type:   fakeStepType{kind: kind},
		Source: spec.SourceSpan{Filename: file, StartLine: line, EndLine: line},
	}
}

func TestDetectDuplicatePromises_NoDuplicates(t *testing.T) {
	ctx := discardCtx(t)
	actions := []spec.Action{
		&mockPromiserAction{kind: "make.node", promises: labels("node:100")},
		&mockPromiserAction{kind: "make.node", promises: labels("node:101")},
	}
	steps := []spec.StepInstance{
		mkStep("make.node", "main.scampi", 10),
		mkStep("make.node", "main.scampi", 20),
	}
	if err := detectDuplicatePromises(ctx, actions, []int{0, 1}, steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectDuplicatePromises_DuplicateLabel(t *testing.T) {
	ctx := discardCtx(t)
	actions := []spec.Action{
		&mockPromiserAction{kind: "make.node", promises: labels("node:100")},
		&mockPromiserAction{kind: "make.node", promises: labels("node:100")},
	}
	steps := []spec.StepInstance{
		mkStep("make.node", "main.scampi", 10),
		mkStep("make.node", "main.scampi", 20),
	}
	err := detectDuplicatePromises(ctx, actions, []int{0, 1}, steps)
	if err == nil {
		t.Fatal("expected error for duplicate label, got nil")
	}
	var abort AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T", err)
	}
	if len(abort.Causes) != 1 {
		t.Fatalf("expected 1 cause, got %d", len(abort.Causes))
	}
	var dup DuplicateResourceError
	if !errors.As(abort.Causes[0], &dup) {
		t.Fatalf("expected DuplicateResourceError, got %T", abort.Causes[0])
	}
	if dup.Resource.Kind != spec.ResourceLabel {
		t.Errorf("kind = %v, want ResourceLabel", dup.Resource.Kind)
	}
	if dup.Resource.Name != "node:100" {
		t.Errorf("name = %q, want node:100", dup.Resource.Name)
	}
	if dup.Source.StartLine != 20 {
		t.Errorf("Source.StartLine = %d, want 20 (the duplicate)", dup.Source.StartLine)
	}
	if dup.OtherSource.StartLine != 10 {
		t.Errorf("OtherSource.StartLine = %d, want 10 (the original)", dup.OtherSource.StartLine)
	}
}

func TestDetectDuplicatePromises_DuplicatePath(t *testing.T) {
	ctx := discardCtx(t)
	actions := []spec.Action{
		&mockPromiserAction{kind: "dir", promises: paths("/etc/foo")},
		&mockPromiserAction{kind: "copy", promises: paths("/etc/foo")},
	}
	steps := []spec.StepInstance{
		mkStep("posix.dir", "main.scampi", 5),
		mkStep("posix.copy", "main.scampi", 12),
	}
	err := detectDuplicatePromises(ctx, actions, []int{0, 1}, steps)
	if err == nil {
		t.Fatal("expected error for duplicate path, got nil")
	}
}

func TestDetectDuplicatePromises_DistinctNodesIndependent(t *testing.T) {
	ctx := discardCtx(t)
	actions := []spec.Action{
		&mockPromiserAction{kind: "make.node", promises: labels("node:100")},
		&mockPromiserAction{kind: "make.node", promises: labels("node:200")},
	}
	steps := []spec.StepInstance{
		mkStep("make.node", "main.scampi", 10),
		mkStep("make.node", "main.scampi", 20),
	}
	if err := detectDuplicatePromises(ctx, actions, []int{0, 1}, steps); err != nil {
		t.Fatalf("unexpected error for distinct labels: %v", err)
	}
}

func TestDetectDuplicatePromises_NonPromiserSkipped(t *testing.T) {
	ctx := discardCtx(t)
	actions := []spec.Action{
		&mockAction{kind: "noop"},
		&mockPromiserAction{kind: "make.node", promises: labels("node:100")},
	}
	steps := []spec.StepInstance{
		mkStep("noop", "main.scampi", 5),
		mkStep("make.node", "main.scampi", 10),
	}
	if err := detectDuplicatePromises(ctx, actions, []int{0, 1}, steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectDuplicatePromises_AllResourceKinds(t *testing.T) {
	cases := []struct {
		name     string
		promises []spec.Resource
		wantKind spec.ResourceKind
	}{
		{"label", labels("node:100"), spec.ResourceLabel},
		{"path", paths("/foo"), spec.ResourcePath},
		{"user", users("alice"), spec.ResourceUser},
		{"group", groups("staff"), spec.ResourceGroup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := discardCtx(t)
			actions := []spec.Action{
				&mockPromiserAction{kind: "x", promises: tc.promises},
				&mockPromiserAction{kind: "x", promises: tc.promises},
			}
			steps := []spec.StepInstance{
				mkStep("x", "main.scampi", 10),
				mkStep("x", "main.scampi", 20),
			}
			err := detectDuplicatePromises(ctx, actions, []int{0, 1}, steps)
			if err == nil {
				t.Fatal("expected duplicate error, got nil")
			}
			var dup DuplicateResourceError
			if !errors.As(err, &dup) {
				if abort, ok := err.(AbortError); ok && len(abort.Causes) > 0 {
					if !errors.As(abort.Causes[0], &dup) {
						t.Fatalf("could not unwrap to DuplicateResourceError")
					}
				}
			}
			if dup.Resource.Kind != tc.wantKind {
				t.Errorf("Resource.Kind = %v, want %v", dup.Resource.Kind, tc.wantKind)
			}
		})
	}
}
