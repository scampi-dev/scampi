// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"fmt"
	"strconv"

	"go.starlark.net/starlark"

	"godoit.dev/doit/spec"
)

// predeclared returns the global builtins available in every .star file.
func predeclared() starlark.StringDict {
	return starlark.StringDict{
		"copy":     starlark.NewBuiltin("copy", builtinCopy),
		"dir":      starlark.NewBuiltin("dir", builtinDir),
		"pkg":      starlark.NewBuiltin("pkg", builtinPkg),
		"run":      starlark.NewBuiltin("run", builtinRun),
		"service":  starlark.NewBuiltin("service", builtinService),
		"symlink":  starlark.NewBuiltin("symlink", builtinSymlink),
		"template": starlark.NewBuiltin("template", builtinTemplate),
		"target":   targetModule(),
		"deploy":   starlark.NewBuiltin("deploy", builtinDeploy),
		"env":      starlark.NewBuiltin("env", builtinEnv),
	}
}

// deploy(name, targets, steps)
// -----------------------------------------------------------------------------

func builtinDeploy(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name    string
		targets *starlark.List
		steps   *starlark.List
	)
	if err := starlark.UnpackArgs("deploy", args, kwargs,
		"name", &name,
		"targets", &targets,
		"steps", &steps,
	); err != nil {
		return nil, err
	}

	span := callSpan(thread)

	if name == "" {
		return nil, &EmptyNameError{Func: "deploy", Source: span}
	}

	targetNames, err := stringList(targets, "deploy", "targets")
	if err != nil {
		return nil, err
	}
	if len(targetNames) == 0 {
		return nil, &EmptyListError{Func: "deploy", Field: "targets", Source: span}
	}

	stepInstances, err := extractSteps(steps, "deploy")
	if err != nil {
		return nil, err
	}
	if len(stepInstances) == 0 {
		return nil, &EmptyListError{Func: "deploy", Field: "steps", Source: span}
	}
	block := spec.DeployBlock{
		Name:    name,
		Targets: targetNames,
		Steps:   stepInstances,
		Source:  span,
	}

	c := threadCollector(thread)
	if err := c.AddDeploy(name, block, span); err != nil {
		return nil, err
	}

	return starlark.None, nil
}

func extractSteps(
	list *starlark.List, fn string,
) ([]spec.StepInstance, error) {
	if list == nil {
		return nil, nil
	}
	out := make([]spec.StepInstance, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		v := list.Index(i)
		step, ok := v.(*StarlarkStep)
		if !ok {
			return nil, &TypeError{
				Context:  fmt.Sprintf("%s: steps[%d]", fn, i),
				Expected: "step",
				Got:      v.Type(),
			}
		}
		inst := step.Instance
		if inst.Desc == "" && inst.Type != nil {
			inst.Desc = fmt.Sprintf("%s[%d]", inst.Type.Kind(), i)
		}
		out = append(out, inst)
	}
	return out, nil
}

// env(key, default?)
// -----------------------------------------------------------------------------

func builtinEnv(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	span := callSpan(thread)

	if len(args) < 1 || len(args) > 2 {
		return nil, &EnvError{
			Detail: fmt.Sprintf("accepts 1 or 2 positional arguments, got %d", len(args)),
			Source: span,
		}
	}
	if len(kwargs) > 0 {
		return nil, &EnvError{
			Detail: "does not accept keyword arguments",
			Source: span,
		}
	}

	key, ok := starlark.AsString(args[0])
	if !ok {
		return nil, &EnvError{
			Detail: fmt.Sprintf("key must be a string, got %s", args[0].Type()),
			Source: span,
		}
	}

	c := threadCollector(thread)
	envVal, found := c.src.LookupEnv(key)

	// No default → required
	if len(args) == 1 {
		if !found {
			return nil, &EnvVarRequiredError{
				Key:    key,
				Source: span,
			}
		}
		return starlark.String(envVal), nil
	}

	// Has default → coerce env value to match default's type
	dflt := args[1]
	if !found {
		return dflt, nil
	}

	return coerceEnvValue(envVal, dflt, span)
}

func coerceEnvValue(
	raw string, dflt starlark.Value, span spec.SourceSpan,
) (starlark.Value, error) {
	switch dflt.(type) {
	case starlark.String:
		return starlark.String(raw), nil

	case starlark.Int:
		i, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, &EnvError{
				Detail: fmt.Sprintf("cannot parse %q as int: %s", raw, err),
				Source: span,
			}
		}
		return starlark.MakeInt64(i), nil

	case starlark.Bool:
		switch raw {
		case "true", "1", "yes":
			return starlark.True, nil
		case "false", "0", "no", "":
			return starlark.False, nil
		default:
			return nil, &EnvError{
				Detail: fmt.Sprintf("cannot parse %q as bool", raw),
				Source: span,
			}
		}

	default:
		return starlark.String(raw), nil
	}
}
