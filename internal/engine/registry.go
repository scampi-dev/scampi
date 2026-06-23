// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"maps"
	"reflect"
	"slices"

	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/container"
	"scampi.dev/scampi/internal/step/copy"
	"scampi.dev/scampi/internal/step/dir"
	"scampi.dev/scampi/internal/step/firewall"
	"scampi.dev/scampi/internal/step/group"
	"scampi.dev/scampi/internal/step/mount"
	"scampi.dev/scampi/internal/step/pkg"
	"scampi.dev/scampi/internal/step/run"
	"scampi.dev/scampi/internal/step/runset"
	"scampi.dev/scampi/internal/step/service"
	"scampi.dev/scampi/internal/step/sharedop"
	"scampi.dev/scampi/internal/step/symlink"
	"scampi.dev/scampi/internal/step/sysctl"
	"scampi.dev/scampi/internal/step/template"
	"scampi.dev/scampi/internal/step/unarchive"
	stepuser "scampi.dev/scampi/internal/step/user"
	"scampi.dev/scampi/internal/target/local"
	"scampi.dev/scampi/internal/target/ssh"
)

type Registry struct {
	stepTypes   map[string]spec.StepType
	targetTypes map[string]spec.TargetType
	converters  map[reflect.Type]spec.TypeConverter
}

func NewRegistry() *Registry {
	stepTypes := []spec.StepType{
		container.Instance{},
		copy.Copy{},
		dir.Dir{},
		firewall.Firewall{},
		group.Group{},
		mount.Mount{},
		pkg.Pkg{},
		run.Run{},
		runset.RunSet{},
		service.Service{},
		sysctl.Sysctl{},
		symlink.Symlink{},
		template.Template{},
		unarchive.Unarchive{},
		stepuser.User{},
	}

	targetTypes := []spec.TargetType{
		local.Local{},
		ssh.SSH{},
	}

	r := &Registry{
		stepTypes:   make(map[string]spec.StepType),
		targetTypes: make(map[string]spec.TargetType),
		converters:  make(map[reflect.Type]spec.TypeConverter),
	}
	for _, st := range stepTypes {
		r.stepTypes[st.Kind()] = st
	}
	for _, t := range targetTypes {
		r.targetTypes[t.Kind()] = t
	}

	// Register type converters from owning packages.
	for _, cm := range []spec.ConverterMap{
		sharedop.Converters(),
		pkg.Converters(),
		container.Converters(),
	} {
		maps.Copy(r.converters, cm)
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
	slices.SortFunc(stepTypes, func(a, b spec.StepType) int {
		if a.Kind() < b.Kind() {
			return -1
		}
		if a.Kind() > b.Kind() {
			return 1
		}
		return 0
	})
	return stepTypes
}

func (r *Registry) ConverterFor(t reflect.Type) (spec.TypeConverter, bool) {
	c, ok := r.converters[t]
	return c, ok
}

func (r *Registry) TargetType(kind string) (spec.TargetType, bool) {
	tgt, ok := r.targetTypes[kind]
	return tgt, ok
}

func (r *Registry) TargetTypes() []spec.TargetType {
	types := make([]spec.TargetType, 0, len(r.targetTypes))
	for _, t := range r.targetTypes {
		types = append(types, t)
	}
	slices.SortFunc(types, func(a, b spec.TargetType) int {
		if a.Kind() < b.Kind() {
			return -1
		}
		if a.Kind() > b.Kind() {
			return 1
		}
		return 0
	})
	return types
}
