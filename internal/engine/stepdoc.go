// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
)

// LoadStepDoc returns documentation for a step type, derived from the
// struct tags on its config struct.
func LoadStepDoc(kind string) spec.StepDoc {
	return loadStepDoc(NewRegistry(), kind)
}

func loadStepDoc(reg *Registry, kind string) spec.StepDoc {
	st, ok := reg.StepType(kind)
	if !ok {
		panic(errs.BUG("loadStepDoc called with unregistered kind: %s", kind))
	}
	return docFromConfig(st.Kind(), st.NewConfig())
}
