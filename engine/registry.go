// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"maps"
	"reflect"
	"slices"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/container"
	"scampi.dev/scampi/step/copy"
	"scampi.dev/scampi/step/dir"
	"scampi.dev/scampi/step/firewall"
	"scampi.dev/scampi/step/group"
	"scampi.dev/scampi/step/mount"
	"scampi.dev/scampi/step/pkg"
	"scampi.dev/scampi/step/pve/lxc"
	steprest "scampi.dev/scampi/step/rest"
	"scampi.dev/scampi/step/run"
	"scampi.dev/scampi/step/service"
	"scampi.dev/scampi/step/sharedop"
	"scampi.dev/scampi/step/symlink"
	"scampi.dev/scampi/step/sysctl"
	"scampi.dev/scampi/step/template"
	"scampi.dev/scampi/step/unarchive"
	stepuser "scampi.dev/scampi/step/user"
	"scampi.dev/scampi/target/local"
	"scampi.dev/scampi/target/pve"
	"scampi.dev/scampi/target/rest"
	"scampi.dev/scampi/target/ssh"
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
		steprest.Request{},
		steprest.Resource{},
		steprest.ResourceSet{},
		lxc.LXC{},
		run.Run{},
		service.Service{},
		sysctl.Sysctl{},
		symlink.Symlink{},
		template.Template{},
		unarchive.Unarchive{},
		stepuser.User{},
	}

	targetTypes := []spec.TargetType{
		local.Local{},
		pve.LXC{},
		rest.REST{},
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
		steprest.Converters(),
		rest.Converters(),
		container.Converters(),
		lxc.Converters(),
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
