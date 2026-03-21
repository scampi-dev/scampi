// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

// LoadStepDoc returns documentation for a step type, derived from the
// struct tags on its config struct.
func LoadStepDoc(kind string) spec.StepDoc {
	reg := NewRegistry()
	st, ok := reg.StepType(kind)
	if !ok {
		panic(errs.BUG("LoadStepDoc called with unregistered kind: %s", kind))
	}
	return docFromConfig(st.Kind(), st.NewConfig())
}
