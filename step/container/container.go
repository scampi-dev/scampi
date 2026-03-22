// SPDX-License-Identifier: GPL-3.0-only

package container

import (
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

// State represents the desired container state.
type State uint8

const (
	StateRunning State = iota + 1
	StateStopped
	StateAbsent
)

func (s State) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StateStopped:
		return "stopped"
	case StateAbsent:
		return "absent"
	default:
		return "unknown"
	}
}

func parseState(s string) State {
	switch s {
	case "running":
		return StateRunning
	case "stopped":
		return StateStopped
	case "absent":
		return StateAbsent
	default:
		panic(errs.BUG("invalid container state %q — should have been caught by validate", s))
	}
}

type (
	Instance       struct{}
	InstanceConfig struct {
		_ struct{} `summary:"Manage container lifecycle: running, stopped, or absent"`

		Desc    string            `step:"Human-readable description" optional:"true"`
		Name    string            `step:"Container name" example:"prometheus"`
		Image   string            `step:"Container image" example:"prom/prometheus:v3.2.0"`
		State   string            `step:"Desired container state" default:"running" example:"stopped|absent"`
		Restart string            `step:"Restart policy" default:"unless-stopped" example:"always|on-failure|no"`
		Ports   []string          `step:"Port mappings (host:container)" optional:"true" example:"[\"9090:9090\"]"`
		Env     map[string]string `step:"Environment variables" optional:"true" example:"{\"DB_HOST\": \"db.local\"}"`
		Mounts  []target.Mount    `step:"Bind mounts (host:container[:ro])" optional:"true" example:"[\"/data:/data\"]"`
		Args    []string          `step:"Arguments for container entrypoint" optional:"true" example:"[\"--verbose\"]"`
		Labels  map[string]string `step:"Container labels" optional:"true" example:"{\"app\": \"myapp\"}"`
	}
	instanceAction struct {
		desc    string
		name    string
		image   string
		state   State
		restart string
		ports   []string
		env     map[string]string
		mounts  []target.Mount
		args    []string
		labels  map[string]string
		step    spec.StepInstance
	}
)

func (Instance) Kind() string   { return "container.instance" }
func (Instance) NewConfig() any { return &InstanceConfig{} }

func (Instance) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*InstanceConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &InstanceConfig{}, step.Config)
	}

	if err := cfg.validate(step); err != nil {
		return nil, err
	}

	return &instanceAction{
		desc:    cfg.Desc,
		name:    cfg.Name,
		image:   cfg.Image,
		state:   parseState(cfg.State),
		restart: cfg.Restart,
		ports:   cfg.Ports,
		env:     cfg.Env,
		mounts:  cfg.Mounts,
		args:    cfg.Args,
		labels:  cfg.Labels,
		step:    step,
	}, nil
}

func (c *InstanceConfig) validate(step spec.StepInstance) error {
	switch c.State {
	case "running", "stopped", "absent":
	default:
		return InvalidStateError{
			Got:     c.State,
			Allowed: []string{"running", "stopped", "absent"},
			Source:  step.Fields["state"].Value,
		}
	}

	switch c.Restart {
	case "always", "on-failure", "unless-stopped", "no":
	default:
		return InvalidRestartError{
			Got:     c.Restart,
			Allowed: []string{"always", "on-failure", "unless-stopped", "no"},
			Source:  step.Fields["restart"].Value,
		}
	}

	if c.State != "absent" && c.Image == "" {
		return EmptyImageError{
			Source: step.Source,
		}
	}

	for _, m := range c.Mounts {
		if m.Source == "" || m.Target == "" {
			return InvalidMountError{
				Got:    m.String(),
				Reason: `must be "host:container" or "host:container:ro"`,
				Source: step.Fields["mounts"].Value,
			}
		}
		if m.Source[0] != '/' {
			return InvalidMountError{
				Got:    m.String(),
				Reason: "host path must be absolute",
				Source: step.Fields["mounts"].Value,
			}
		}
		if m.Target[0] != '/' {
			return InvalidMountError{
				Got:    m.String(),
				Reason: "container path must be absolute",
				Source: step.Fields["mounts"].Value,
			}
		}
	}

	for k := range c.Labels {
		if k == "" {
			return InvalidLabelError{
				Key:    k,
				Reason: "label key must not be empty",
				Source: step.Fields["labels"].Value,
			}
		}
		if strings.ContainsAny(k, " \t") {
			return InvalidLabelError{
				Key:    k,
				Reason: "label key must not contain whitespace",
				Source: step.Fields["labels"].Value,
			}
		}
	}

	return nil
}

func (a *instanceAction) Desc() string { return a.desc }
func (a *instanceAction) Kind() string { return "container.instance" }

func (a *instanceAction) Inputs() []spec.Resource {
	var r []spec.Resource
	for _, m := range a.mounts {
		r = append(r, spec.PathResource(m.Source))
	}
	return r
}

func (a *instanceAction) Promises() []spec.Resource { return nil }

func (a *instanceAction) Ops() []spec.Op {
	op := &ensureContainerOp{
		name:    a.name,
		image:   a.image,
		state:   a.state,
		restart: a.restart,
		ports:   a.ports,
		env:     a.env,
		mounts:  a.mounts,
		args:    a.args,
		labels:  a.labels,
		step:    a.step,
	}
	op.SetAction(a)
	return []spec.Op{op}
}
