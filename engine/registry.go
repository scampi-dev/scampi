// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/copy"
	"scampi.dev/scampi/step/dir"
	"scampi.dev/scampi/step/firewall"
	"scampi.dev/scampi/step/group"
	"scampi.dev/scampi/step/pkg"
	"scampi.dev/scampi/step/run"
	"scampi.dev/scampi/step/service"
	"scampi.dev/scampi/step/symlink"
	"scampi.dev/scampi/step/sysctl"
	"scampi.dev/scampi/step/template"
	stepuser "scampi.dev/scampi/step/user"
	"scampi.dev/scampi/target/local"
	"scampi.dev/scampi/target/ssh"
)

type Registry struct {
	stepTypes   map[string]spec.StepType
	targetTypes map[string]spec.TargetType
}

func NewRegistry() *Registry {
	stepTypes := []spec.StepType{
		copy.Copy{},
		dir.Dir{},
		firewall.Firewall{},
		group.Group{},
		pkg.Pkg{},
		run.Run{},
		service.Service{},
		sysctl.Sysctl{},
		symlink.Symlink{},
		template.Template{},
		stepuser.User{},
	}

	targetTypes := []spec.TargetType{
		local.Local{},
		ssh.SSH{},
	}

	r := &Registry{
		stepTypes:   make(map[string]spec.StepType),
		targetTypes: make(map[string]spec.TargetType),
	}
	for _, st := range stepTypes {
		r.stepTypes[st.Kind()] = st
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
	tgt, ok := r.targetTypes[kind]
	return tgt, ok
}
