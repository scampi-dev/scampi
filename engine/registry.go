package engine

import (
	"godoit.dev/doit/spec"
	"godoit.dev/doit/step/copy"
	"godoit.dev/doit/step/symlink"
	"godoit.dev/doit/step/template"
	"godoit.dev/doit/target/local"
	"godoit.dev/doit/target/ssh"
)

type Registry struct {
	stepTypes   map[string]spec.StepType
	targetTypes map[string]spec.TargetType
}

func NewRegistry() *Registry {
	// TODO: this probably needs to be automatic at some point
	// also: this would be where we need to put extensions
	// for now (probably a while) this is just a manual list

	stepTypes := []spec.StepType{
		copy.Copy{},
		symlink.Symlink{},
		template.Template{},
	}

	targetTypes := []spec.TargetType{
		local.Local{},
		ssh.SSH{},
	}

	r := &Registry{
		stepTypes:   make(map[string]spec.StepType),
		targetTypes: make(map[string]spec.TargetType),
	}
	for _, spec := range stepTypes {
		r.stepTypes[spec.Kind()] = spec
	}
	for _, t := range targetTypes {
		r.targetTypes[t.Kind()] = t
	}

	return r
}

func (r *Registry) StepType(kind string) (spec.StepType, bool) {
	step, ok := r.stepTypes[kind]
	return step, ok
}

func (r *Registry) StepTypes() []spec.StepType {
	stepTypes := make([]spec.StepType, 0, len(r.stepTypes))
	for _, stepType := range r.stepTypes {
		stepTypes = append(stepTypes, stepType)
	}
	return stepTypes
}

func (r *Registry) TargetType(kind string) (spec.TargetType, bool) {
	step, ok := r.targetTypes[kind]
	return step, ok
}
