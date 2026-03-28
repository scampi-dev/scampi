// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"go.starlark.net/starlark"

	"scampi.dev/scampi/spec"
	steprest "scampi.dev/scampi/step/rest"
)

type refValue struct {
	ref spec.Ref
}

func (r *refValue) String() string        { return "<ref>" }
func (r *refValue) Type() string          { return "ref" }
func (r *refValue) Freeze()               {}
func (r *refValue) Truth() starlark.Bool  { return starlark.True }
func (r *refValue) Hash() (uint32, error) { return 0, &UnhashableTypeError{TypeName: "ref"} }

// ref(step, expr)
func builtinRef(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		stepVal starlark.Value
		expr    string
	)
	if err := starlark.UnpackArgs("ref", args, kwargs,
		"step", &stepVal,
		"expr", &expr,
	); err != nil {
		return nil, err
	}

	step, ok := stepVal.(*StarlarkStep)
	if !ok {
		span := callSpan(thread)
		return nil, &TypeError{
			Context:  "ref: step",
			Expected: "a step value (e.g. rest.resource(...))",
			Got:      stepVal.Type(),
			Source:   span,
		}
	}

	if _, err := steprest.CompileJQ(expr); err != nil {
		span := callSpan(thread)
		return nil, &JQCompileError{Expr: expr, Source: span, Err: err}
	}

	return &refValue{ref: spec.Ref{
		TargetID: step.Instance.ID,
		Expr:     expr,
		Source:   callSpan(thread),
	}}, nil
}
