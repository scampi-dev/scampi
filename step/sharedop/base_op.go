// SPDX-License-Identifier: GPL-3.0-only

package sharedop

import "scampi.dev/scampi/spec"

type BaseOp struct {
	SrcSpan  spec.SourceSpan
	DestSpan spec.SourceSpan
	action   spec.Action
	deps     []spec.Op
}

func (op *BaseOp) Action() spec.Action          { return op.action }
func (op *BaseOp) DependsOn() []spec.Op         { return op.deps }
func (op *BaseOp) SetAction(action spec.Action) { op.action = action }
func (op *BaseOp) AddDependency(dep spec.Op)    { op.deps = append(op.deps, dep) }
