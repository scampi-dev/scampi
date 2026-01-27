package engine

import (
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/copy"
	"godoit.dev/doit/step/symlink"
)

type Registry struct {
	types map[string]spec.StepType
}

func NewRegistry() *Registry {
	// TODO: this probably needs to be automatic at some point
	// also: this would be where we need to put extensions
	// for now (probably a while) this is just a manual list
	types := []spec.StepType{
		copy.Copy{},
		symlink.Symlink{},
	}

	r := &Registry{}
	r.types = make(map[string]spec.StepType)
	for _, spec := range types {
		r.types[spec.Kind()] = spec
	}

	return r
}

func (r *Registry) StepType(kind string) (spec.StepType, bool) {
	step, ok := r.types[kind]
	return step, ok
}

// StepTypes returns all registered step types.
func (r *Registry) StepTypes() []spec.StepType {
	stepTypes := make([]spec.StepType, 0, len(r.types))
	for _, stepType := range r.types {
		stepTypes = append(stepTypes, stepType)
	}
	return stepTypes
}
