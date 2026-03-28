// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"strings"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/container"
	"scampi.dev/scampi/target"
)

// containerModule builds the `container` namespace (container.instance).
func containerModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "container",
		Members: starlark.StringDict{
			"instance": starlark.NewBuiltin("container.instance", builtinContainerInstance),
			"healthcheck": &starlarkstruct.Module{
				Name: "container.healthcheck",
				Members: starlark.StringDict{
					"cmd": starlark.NewBuiltin("container.healthcheck.cmd", builtinHealthcheckCmd),
				},
			},
		},
	}
}

// container.instance(name, image, state?, restart?, ports?, env?, mounts?, desc?, on_change?)
func builtinContainerInstance(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name           string
		image          string
		state          = "running"
		restart        = "unless-stopped"
		portsVal       *starlark.List
		envVal         *starlark.Dict
		mountsVal      *starlark.List
		argsVal        *starlark.List
		labelsVal      *starlark.Dict
		healthcheckVal starlark.Value
		desc           string
		onChangeVal    starlark.Value
	)
	if err := starlark.UnpackArgs("container.instance", args, kwargs,
		"name", &name,
		"image?", &image,
		"state?", &state,
		"restart?", &restart,
		"ports?", &portsVal,
		"env?", &envVal,
		"mounts?", &mountsVal,
		"args?", &argsVal,
		"labels?", &labelsVal,
		"healthcheck?", &healthcheckVal,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "container.instance")
	if err != nil {
		return nil, err
	}

	var ports []target.Port
	if portsVal != nil {
		raw, parseErr := stringList(portsVal, "container.instance", "ports")
		if parseErr != nil {
			return nil, parseErr
		}
		for _, s := range raw {
			ports = append(ports, parsePort(s))
		}
	}

	var env map[string]string
	if envVal != nil {
		env, err = starlarkDictToStringMap(envVal, "container.instance env")
		if err != nil {
			return nil, err
		}
	}

	var mounts []target.Mount
	if mountsVal != nil {
		raw, parseErr := stringList(mountsVal, "container.instance", "mounts")
		if parseErr != nil {
			return nil, parseErr
		}
		for _, s := range raw {
			mounts = append(mounts, parseMount(s))
		}
	}

	var ctrArgs []string
	if argsVal != nil {
		ctrArgs, err = stringList(argsVal, "container.instance", "args")
		if err != nil {
			return nil, err
		}
	}

	var labels map[string]string
	if labelsVal != nil {
		labels, err = starlarkDictToStringMap(labelsVal, "container.instance labels")
		if err != nil {
			return nil, err
		}
	}

	var hc *target.Healthcheck
	if healthcheckVal != nil && healthcheckVal != starlark.None {
		shc, ok := healthcheckVal.(*StarlarkHealthcheck)
		if !ok {
			return nil, &TypeError{
				Context:  "container.instance: healthcheck",
				Expected: "container.healthcheck.cmd(...)",
				Got:      healthcheckVal.Type(),
				Source:   resolveArgSpan(thread, "healthcheck"),
			}
		}
		hc = &shc.HC
	}

	span := callSpan(thread)
	fieldNames := []string{
		"name", "image", "state", "restart", "ports", "env",
		"mounts", "args", "labels", "healthcheck",
	}
	fields := kwargsFieldSpans(thread, fieldNames...)

	return newStarlarkStep(spec.StepInstance{
		Desc: desc,
		Type: container.Instance{},
		Config: &container.InstanceConfig{
			Desc: desc, Name: name, Image: image,
			State: state, Restart: restart, Ports: ports,
			Env: env, Mounts: mounts, Args: ctrArgs,
			Labels: labels, Healthcheck: hc,
		},
		OnChange: hookIDs,
		Source:   span,
		Fields:   fields,
	}), nil
}

func parseMount(s string) target.Mount {
	parts := strings.SplitN(s, ":", 3)
	m := target.Mount{}
	if len(parts) >= 1 {
		m.Source = parts[0]
	}
	if len(parts) >= 2 {
		m.Target = parts[1]
	}
	if len(parts) == 3 && parts[2] == "ro" {
		m.ReadOnly = true
	}
	return m
}

// parsePort parses port formats:
//   - "hostPort:containerPort"
//   - "hostPort:containerPort/proto"
//   - "ip:hostPort:containerPort"
//   - "ip:hostPort:containerPort/proto"
func parsePort(s string) target.Port {
	p := target.Port{Proto: target.ProtoTCP}

	// Split off /proto suffix
	if base, proto, ok := strings.Cut(s, "/"); ok {
		s = base
		p.Proto = target.ParsePortProto(proto)
	}

	parts := strings.SplitN(s, ":", 3)
	switch len(parts) {
	case 2:
		p.HostPort = parts[0]
		p.ContainerPort = parts[1]
	case 3:
		p.HostIP = parts[0]
		p.HostPort = parts[1]
		p.ContainerPort = parts[2]
	default:
		p.ContainerPort = s
	}
	return p
}

// StarlarkHealthcheck
// -----------------------------------------------------------------------------

type StarlarkHealthcheck struct {
	HC target.Healthcheck
}

func (s *StarlarkHealthcheck) String() string        { return "container.healthcheck.cmd(...)" }
func (s *StarlarkHealthcheck) Type() string          { return "healthcheck" }
func (s *StarlarkHealthcheck) Freeze()               {}
func (s *StarlarkHealthcheck) Truth() starlark.Bool  { return true }
func (s *StarlarkHealthcheck) Hash() (uint32, error) { return 0, nil }

func builtinHealthcheckCmd(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		cmd         string
		interval    string
		timeout     string
		retries     int
		startPeriod string
	)
	if err := starlark.UnpackArgs("container.healthcheck.cmd", args, kwargs,
		"cmd", &cmd,
		"interval?", &interval,
		"timeout?", &timeout,
		"retries?", &retries,
		"start_period?", &startPeriod,
	); err != nil {
		return nil, err
	}

	hc := target.Healthcheck{
		Cmd:      cmd,
		Interval: 30 * time.Second,
		Timeout:  30 * time.Second,
		Retries:  3,
	}

	if retries > 0 {
		hc.Retries = retries
	}
	if interval != "" {
		d, err := time.ParseDuration(interval)
		if err != nil {
			return nil, &InvalidDurationError{
				Field:  "interval",
				Got:    interval,
				Source: resolveArgSpan(thread, "interval"),
			}
		}
		hc.Interval = d
	}
	if timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, &InvalidDurationError{
				Field:  "timeout",
				Got:    timeout,
				Source: resolveArgSpan(thread, "timeout"),
			}
		}
		hc.Timeout = d
	}
	if startPeriod != "" {
		d, err := time.ParseDuration(startPeriod)
		if err != nil {
			return nil, &InvalidDurationError{
				Field:  "start_period",
				Got:    startPeriod,
				Source: resolveArgSpan(thread, "start_period"),
			}
		}
		hc.StartPeriod = d
	}

	return &StarlarkHealthcheck{HC: hc}, nil
}
