// SPDX-License-Identifier: GPL-3.0-only

package engine

import "godoit.dev/doit/spec"

// LoadStepDoc returns documentation for a step type, derived from the
// struct tags on its config struct.
func LoadStepDoc(kind string) spec.StepDoc {
	reg := NewRegistry()
	st, ok := reg.StepType(kind)
	if !ok {
		panic("LoadStepDoc called with unregistered kind: " + kind)
	}
	return spec.DocFromConfig(st.Kind(), st.NewConfig())
}
