package engine

import (
	"godoit.dev/doit/kinds"
	"godoit.dev/doit/spec"
)

type Registry struct {
	types map[string]spec.UnitType
}

func NewRegistry() (*Registry, error) {
	// TODO: this probably needs to be automatic at some point
	// also: this would be where we need to put extensions
	// for now (probably a while) this is just a manual list
	types := []spec.UnitType{
		kinds.CopySpec{},
	}

	r := &Registry{}
	r.types = make(map[string]spec.UnitType)
	for _, spec := range types {
		r.types[spec.Kind()] = spec
	}

	return r, nil
}

func (r *Registry) Type(kind string) (spec.UnitType, bool) {
	spec, ok := r.types[kind]
	return spec, ok
}
