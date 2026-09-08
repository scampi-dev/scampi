// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"testing"

	"scampi.dev/scampi/internal/capability"
	"scampi.dev/scampi/internal/controller"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/target"
)

// orderOp is a minimal op for plan-shape tests: identity + deps, no behavior.
type orderOp struct {
	deps []spec.Op
}

func (o *orderOp) Step() spec.Step { return nil }

func (o *orderOp) Check(
	context.Context,
	controller.Controller,
	target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	return spec.CheckUnknown, nil, nil
}

func (o *orderOp) Execute(context.Context, controller.Controller, target.Target) (spec.Result, error) {
	return spec.Result{}, nil
}

func (o *orderOp) DependsOn() []spec.Op                        { return o.deps }
func (o *orderOp) RequiredCapabilities() capability.Capability { return capability.None }

// buildPlan must return nodes in declaration order: StepReport.Ops and
// event.Result.Ops inherit it, so it is a determinism contract, not an
// implementation detail (#440). Map iteration order passing by luck is
// probabilistic, so hammer it.
func Test_BuildPlan_PreservesDeclarationOrder(t *testing.T) {
	for range 50 {
		ops := make([]spec.Op, 12)
		for i := range ops {
			op := &orderOp{}
			if i > 0 {
				// A shared dep so order survives graphs with edges, not just
				// independent ops.
				op.deps = []spec.Op{ops[0]}
			}
			ops[i] = op
		}

		nodes, err := buildPlan(ops)
		if err != nil {
			t.Fatalf("buildPlan: %v", err)
		}
		if len(nodes) != len(ops) {
			t.Fatalf("node count: got %d, want %d", len(nodes), len(ops))
		}
		for i, n := range nodes {
			if n.op != ops[i] {
				t.Fatalf("node %d out of declaration order", i)
			}
		}
	}
}
