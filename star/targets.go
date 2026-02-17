// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"godoit.dev/doit/spec"
	"godoit.dev/doit/target/local"
	"godoit.dev/doit/target/ssh"
)

// targetModule builds the `target` namespace (target.ssh, target.local).
func targetModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "target",
		Members: starlark.StringDict{
			"ssh":   starlark.NewBuiltin("target.ssh", builtinTargetSSH),
			"local": starlark.NewBuiltin("target.local", builtinTargetLocal),
		},
	}
}

// target.ssh(name, host, user, ...)
func builtinTargetSSH(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name     string
		host     string
		user     string
		port     int
		key      string
		insecure bool
		timeout  string
	)
	if err := starlark.UnpackArgs("target.ssh", args, kwargs,
		"name", &name,
		"host", &host,
		"user", &user,
		"port?", &port,
		"key?", &key,
		"insecure?", &insecure,
		"timeout?", &timeout,
	); err != nil {
		return nil, err
	}

	span := callSpan(thread)
	pos := callerPosition(thread)
	call := findCallFromThread(thread, pos)

	if name == "" {
		s := span
		if call != nil {
			if vs, ok := kwargValueSpan(call, "name"); ok {
				s = vs
			}
		}
		return nil, &EmptyNameError{Func: "target.ssh", Source: s}
	}
	fields := kwargsFieldSpans(thread,
		"host", "user", "port", "key", "insecure", "timeout")

	inst := spec.TargetInstance{
		Type: ssh.SSH{},
		Config: &ssh.Config{
			Host: host, Port: port, User: user,
			Key: key, Insecure: insecure, Timeout: timeout,
		},
		Source: span,
		Fields: fields,
	}

	c := threadCollector(thread)
	if err := c.AddTarget(name, inst, span); err != nil {
		return nil, err
	}

	return poisonValue{funcName: "target.ssh"}, nil
}

// target.local(name)
func builtinTargetLocal(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("target.local", args, kwargs,
		"name", &name,
	); err != nil {
		return nil, err
	}

	span := callSpan(thread)
	pos := callerPosition(thread)
	call := findCallFromThread(thread, pos)

	if name == "" {
		s := span
		if call != nil {
			if vs, ok := kwargValueSpan(call, "name"); ok {
				s = vs
			}
		}
		return nil, &EmptyNameError{Func: "target.local", Source: s}
	}
	inst := spec.TargetInstance{
		Type:   local.Local{},
		Config: &local.Config{},
		Source: span,
		Fields: make(map[string]spec.FieldSpan),
	}

	c := threadCollector(thread)
	if err := c.AddTarget(name, inst, span); err != nil {
		return nil, err
	}

	return poisonValue{funcName: "target.local"}, nil
}
