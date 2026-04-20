// SPDX-License-Identifier: GPL-3.0-only

package spec

import "fmt"

// StepID is a unique identifier assigned to each step during scampi
// evaluation. Used internally for ref() targeting and output registry
// keying — never exposed to the user.
type StepID uint64

// Ref is a runtime value reference from one step to another's settled
// state. Created by the ref() scampi builtin, it survives in
// map[string]any configs and is resolved by the engine at execution time.
type Ref struct {
	TargetID StepID     // step to reference
	Expr     string     // jq expression to evaluate against the step's output
	Source   SourceSpan // call site of ref() for error reporting
}

// RefResolver resolves a Ref to a concrete value. The engine provides
// the implementation; steps call it during ResolveRefs.
type RefResolver func(ref Ref) (any, error)

// RefResource creates a Resource identifying a step's output by its ID.
func RefResource(id StepID) Resource {
	return Resource{Kind: ResourceRef, Name: fmt.Sprint(id)}
}
