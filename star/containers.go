// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"strings"

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
		name        string
		image       string
		state       = "running"
		restart     = "unless-stopped"
		portsVal    *starlark.List
		envVal      *starlark.Dict
		mountsVal   *starlark.List
		argsVal     *starlark.List
		labelsVal   *starlark.Dict
		desc        string
		onChangeVal starlark.Value
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
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "container.instance")
	if err != nil {
		return nil, err
	}

	var ports []string
	if portsVal != nil {
		ports, err = stringList(portsVal, "container.instance", "ports")
		if err != nil {
			return nil, err
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

	span := callSpan(thread)
	fields := kwargsFieldSpans(thread, "name", "image", "state", "restart", "ports", "env", "mounts", "args", "labels")

	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: container.Instance{},
			Config: &container.InstanceConfig{
				Desc: desc, Name: name, Image: image,
				State: state, Restart: restart, Ports: ports,
				Env: env, Mounts: mounts, Args: ctrArgs,
				Labels: labels,
			},
			OnChange: hookIDs,
			Source:   span,
			Fields:   fields,
		},
	}, nil
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
