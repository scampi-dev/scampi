// SPDX-License-Identifier: GPL-3.0-only

package sharedop

import "scampi.dev/scampi/internal/spec"

type BaseOp struct {
	SrcSpan  spec.SourceSpan
	DestSpan spec.SourceSpan
	step     spec.Step
	deps     []spec.Op
}

func (op *BaseOp) Step() spec.Step           { return op.step }
func (op *BaseOp) DependsOn() []spec.Op      { return op.deps }
func (op *BaseOp) SetStep(step spec.Step)    { op.step = step }
func (op *BaseOp) AddDependency(dep spec.Op) { op.deps = append(op.deps, dep) }
