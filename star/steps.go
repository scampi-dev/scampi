// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"fmt"

	"go.starlark.net/starlark"

	"scampi.dev/scampi/spec"
	stepcopy "scampi.dev/scampi/step/copy"
	"scampi.dev/scampi/step/dir"
	"scampi.dev/scampi/step/firewall"
	"scampi.dev/scampi/step/group"
	"scampi.dev/scampi/step/pkg"
	"scampi.dev/scampi/step/run"
	"scampi.dev/scampi/step/service"
	"scampi.dev/scampi/step/symlink"
	"scampi.dev/scampi/step/sysctl"
	"scampi.dev/scampi/step/template"
	"scampi.dev/scampi/step/unarchive"
	stepuser "scampi.dev/scampi/step/user"
)

// StarlarkStep wraps a spec.StepInstance as an opaque Starlark value.
type StarlarkStep struct {
	Instance spec.StepInstance
}

func (s *StarlarkStep) String() string {
	kind := "step"
	if s.Instance.Type != nil {
		kind = s.Instance.Type.Kind()
	}
	if s.Instance.Desc != "" {
		return fmt.Sprintf("<%s %q>", kind, s.Instance.Desc)
	}
	return fmt.Sprintf("<%s>", kind)
}

func (s *StarlarkStep) Type() string         { return "step" }
func (s *StarlarkStep) Freeze()              {}
func (s *StarlarkStep) Truth() starlark.Bool { return starlark.True }

func (s *StarlarkStep) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: step")
}

// Step builtin: copy
// -----------------------------------------------------------------------------

func builtinCopy(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		srcVal      starlark.Value
		dest        string
		perm        string
		owner       string
		group       string
		verify      string
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("copy", args, kwargs,
		"src", &srcVal,
		"dest", &dest,
		"perm", &perm,
		"owner", &owner,
		"group", &group,
		"verify?", &verify,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	srcRef, err := unpackSourceRef(srcVal, "copy")
	if err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "copy")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: stepcopy.Copy{},
			Config: &stepcopy.CopyConfig{
				Desc: desc, Src: srcRef,
				Dest: dest, Perm: perm, Owner: owner, Group: group,
				Verify: verify,
			},
			OnChange: hookIDs,
			Source:   span,
			Fields: kwargsFieldSpans(
				thread,
				"src",
				"dest",
				"perm",
				"owner",
				"group",
				"verify",
				"on_change",
			),
		},
	}, nil
}

// Step builtin: dir
// -----------------------------------------------------------------------------

func builtinDir(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		path        string
		perm        string
		owner       string
		group       string
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("dir", args, kwargs,
		"path", &path,
		"perm?", &perm,
		"owner?", &owner,
		"group?", &group,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "dir")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc:     desc,
			Type:     dir.Dir{},
			Config:   &dir.DirConfig{Desc: desc, Path: path, Perm: perm, Owner: owner, Group: group},
			OnChange: hookIDs,
			Source:   span,
			Fields:   kwargsFieldSpans(thread, "path", "perm", "owner", "group", "on_change"),
		},
	}, nil
}

// Step builtin: firewall
// -----------------------------------------------------------------------------

func builtinFirewall(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		port        string
		action      = "allow"
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("firewall", args, kwargs,
		"port", &port,
		"action?", &action,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "firewall")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: firewall.Firewall{},
			Config: &firewall.FirewallConfig{
				Desc: desc, Port: port, Action: action,
			},
			OnChange: hookIDs,
			Source:   span,
			Fields:   kwargsFieldSpans(thread, "port", "action", "on_change"),
		},
	}, nil
}

// Step builtin: pkg
// -----------------------------------------------------------------------------

func builtinPkg(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		packages    *starlark.List
		state       = "present"
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("pkg", args, kwargs,
		"packages", &packages,
		"state?", &state,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	pkgs, err := stringList(packages, "pkg", "packages")
	if err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "pkg")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc:     desc,
			Type:     pkg.Pkg{},
			Config:   &pkg.PkgConfig{Desc: desc, Packages: pkgs, State: state},
			OnChange: hookIDs,
			Source:   span,
			Fields:   kwargsFieldSpans(thread, "packages", "state", "on_change"),
		},
	}, nil
}

// Step builtin: run
// -----------------------------------------------------------------------------

func builtinRun(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		apply       string
		check       string
		always      bool
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("run", args, kwargs,
		"apply", &apply,
		"check?", &check,
		"always?", &always,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "run")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc:     desc,
			Type:     run.Run{},
			Config:   &run.RunConfig{Desc: desc, Apply: apply, Check: check, Always: always},
			OnChange: hookIDs,
			Source:   span,
			Fields:   kwargsFieldSpans(thread, "apply", "check", "always", "on_change"),
		},
	}, nil
}

// Step builtin: service
// -----------------------------------------------------------------------------

func builtinService(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name        string
		state       = "running"
		enabled     = true
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("service", args, kwargs,
		"name", &name,
		"state?", &state,
		"enabled?", &enabled,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "service")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc:     desc,
			Type:     service.Service{},
			Config:   &service.ServiceConfig{Desc: desc, Name: name, State: state, Enabled: enabled},
			OnChange: hookIDs,
			Source:   span,
			Fields:   kwargsFieldSpans(thread, "name", "state", "enabled", "on_change"),
		},
	}, nil
}

// Step builtin: sysctl
// -----------------------------------------------------------------------------

func builtinSysctl(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		key         string
		value       string
		persist     = true
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("sysctl", args, kwargs,
		"key", &key,
		"value", &value,
		"persist?", &persist,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "sysctl")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: sysctl.Sysctl{},
			Config: &sysctl.SysctlConfig{
				Desc: desc, Key: key, Value: value, Persist: persist,
			},
			OnChange: hookIDs,
			Source:   span,
			Fields:   kwargsFieldSpans(thread, "key", "value", "persist", "on_change"),
		},
	}, nil
}

// Step builtin: symlink
// -----------------------------------------------------------------------------

func builtinSymlink(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		target      string
		link        string
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("symlink", args, kwargs,
		"target", &target,
		"link", &link,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "symlink")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc:     desc,
			Type:     symlink.Symlink{},
			Config:   &symlink.SymlinkConfig{Desc: desc, Target: target, Link: link},
			OnChange: hookIDs,
			Source:   span,
			Fields:   kwargsFieldSpans(thread, "target", "link", "on_change"),
		},
	}, nil
}

// Step builtin: template
// -----------------------------------------------------------------------------

func builtinTemplate(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		srcVal      starlark.Value
		dest        string
		perm        string
		owner       string
		group       string
		data        *starlark.Dict
		verify      string
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("template", args, kwargs,
		"src", &srcVal,
		"dest", &dest,
		"perm", &perm,
		"owner", &owner,
		"group", &group,
		"data?", &data,
		"verify?", &verify,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	srcRef, err := unpackSourceRef(srcVal, "template")
	if err != nil {
		return nil, err
	}

	dataCfg, err := convertDataConfig(data)
	if err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "template")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: template.Template{},
			Config: &template.TemplateConfig{
				Desc: desc, Src: srcRef, Dest: dest,
				Data: dataCfg, Perm: perm, Owner: owner, Group: group,
				Verify: verify,
			},
			OnChange: hookIDs,
			Source:   span,
			Fields: kwargsFieldSpans(
				thread,
				"src",
				"dest",
				"perm",
				"owner",
				"group",
				"verify",
				"on_change",
			),
		},
	}, nil
}

// Step builtin: unarchive
// -----------------------------------------------------------------------------

func builtinUnarchive(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		srcVal      starlark.Value
		dest        string
		depth       = -1
		owner       string
		group       string
		perm        string
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("unarchive", args, kwargs,
		"src", &srcVal,
		"dest", &dest,
		"depth?", &depth,
		"owner?", &owner,
		"group?", &group,
		"perm?", &perm,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	srcRef, err := unpackSourceRef(srcVal, "unarchive")
	if err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "unarchive")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: unarchive.Unarchive{},
			Config: &unarchive.UnarchiveConfig{
				Desc: desc, Src: srcRef, Dest: dest,
				Depth: depth, Owner: owner, Group: group,
				Perm: perm,
			},
			OnChange: hookIDs,
			Source:   span,
			Fields: kwargsFieldSpans(
				thread,
				"src",
				"dest",
				"depth",
				"owner",
				"group",
				"perm",
				"on_change",
			),
		},
	}, nil
}

// Step builtin: user
// -----------------------------------------------------------------------------

func builtinUser(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name        string
		state       = "present"
		shell       string
		home        string
		system      bool
		password    string
		groups      *starlark.List
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("user", args, kwargs,
		"name", &name,
		"state?", &state,
		"shell?", &shell,
		"home?", &home,
		"system?", &system,
		"password?", &password,
		"groups?", &groups,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	groupList, err := stringList(groups, "user", "groups")
	if err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "user")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: stepuser.User{},
			Config: &stepuser.UserConfig{
				Desc: desc, Name: name, State: state,
				Shell: shell, Home: home, System: system,
				Password: password, Groups: groupList,
			},
			OnChange: hookIDs,
			Source:   span,
			Fields: kwargsFieldSpans(
				thread,
				"name",
				"state",
				"shell",
				"home",
				"system",
				"password",
				"groups",
				"on_change",
			),
		},
	}, nil
}

// Step builtin: group
// -----------------------------------------------------------------------------

func builtinGroup(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name        string
		state       = "present"
		gid         int
		system      bool
		desc        string
		onChangeVal starlark.Value
	)
	if err := starlark.UnpackArgs("group", args, kwargs,
		"name", &name,
		"state?", &state,
		"gid?", &gid,
		"system?", &system,
		"desc?", &desc,
		"on_change?", &onChangeVal,
	); err != nil {
		return nil, err
	}

	hookIDs, err := unpackOnChange(thread, onChangeVal, "group")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: group.Group{},
			Config: &group.GroupConfig{
				Desc: desc, Name: name, State: state,
				GID: gid, System: system,
			},
			OnChange: hookIDs,
			Source:   span,
			Fields:   kwargsFieldSpans(thread, "name", "state", "gid", "system", "on_change"),
		},
	}, nil
}

// Helpers
// -----------------------------------------------------------------------------

func unpackSourceRef(val starlark.Value, fn string) (spec.SourceRef, error) {
	src, ok := val.(*StarlarkSource)
	if !ok {
		return spec.SourceRef{}, &TypeError{
			Context:  fn + ": src",
			Expected: `source resolver, e.g. local("./path") or inline("content")`,
			Got:      val.Type(),
		}
	}
	return src.Ref, nil
}

func unpackOnChange(thread *starlark.Thread, val starlark.Value, fn string) ([]string, error) {
	if val == nil || val == starlark.None {
		return nil, nil
	}
	if s, ok := starlark.AsString(val); ok {
		return []string{s}, nil
	}
	list, ok := val.(*starlark.List)
	if !ok {
		source := callSpan(thread)
		pos := callerPosition(thread)
		if call := findCallFromThread(thread, pos); call != nil {
			if vs, ok := kwargValueSpan(call, "on_change"); ok {
				source = vs
			}
		}
		return nil, &TypeError{
			Context:  fmt.Sprintf("%s: on_change", fn),
			Expected: "string or list of strings",
			Got:      val.Type(),
			Source:   source,
		}
	}
	return stringList(list, fn, "on_change")
}

func stringList(list *starlark.List, fn, arg string) ([]string, error) {
	if list == nil {
		return nil, nil
	}
	out := make([]string, list.Len())
	for i := 0; i < list.Len(); i++ {
		s, ok := starlark.AsString(list.Index(i))
		if !ok {
			return nil, &TypeError{
				Context:  fmt.Sprintf("%s: %s[%d]", fn, arg, i),
				Expected: "string",
				Got:      list.Index(i).Type(),
			}
		}
		out[i] = s
	}
	return out, nil
}

func convertDataConfig(data *starlark.Dict) (template.DataConfig, error) {
	if data == nil {
		return template.DataConfig{}, nil
	}

	dc := template.DataConfig{}
	for _, item := range data.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return dc, &TypeError{
				Context:  "data key",
				Expected: "string",
				Got:      item[0].Type(),
			}
		}

		switch key {
		case "values":
			dict, ok := item[1].(*starlark.Dict)
			if !ok {
				return dc, &TypeError{
					Context:  "data.values",
					Expected: "dict",
					Got:      item[1].Type(),
				}
			}
			vals, err := starlarkDictToMap(dict, "data.values")
			if err != nil {
				return dc, err
			}
			dc.Values = vals

		case "env":
			dict, ok := item[1].(*starlark.Dict)
			if !ok {
				return dc, &TypeError{
					Context:  "data.env",
					Expected: "dict",
					Got:      item[1].Type(),
				}
			}
			envMap, err := starlarkDictToStringMap(dict, "data.env")
			if err != nil {
				return dc, err
			}
			dc.Env = envMap

		default:
			return dc, &UnknownKeyError{
				Key:     key,
				Allowed: []string{"values", "env"},
			}
		}
	}

	return dc, nil
}

func starlarkDictToMap(dict *starlark.Dict, ctx string) (map[string]any, error) {
	result := make(map[string]any, dict.Len())
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, &TypeError{
				Context:  ctx + " key",
				Expected: "string",
				Got:      item[0].Type(),
			}
		}
		if err := checkPoison(item[1]); err != nil {
			return nil, err
		}
		result[key] = starlarkToGo(item[1])
	}
	return result, nil
}

func starlarkDictToStringMap(dict *starlark.Dict, ctx string) (map[string]string, error) {
	result := make(map[string]string, dict.Len())
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, &TypeError{
				Context:  ctx + " key",
				Expected: "string",
				Got:      item[0].Type(),
			}
		}
		val, ok := starlark.AsString(item[1])
		if !ok {
			return nil, &TypeError{
				Context:  fmt.Sprintf("%s value for %q", ctx, key),
				Expected: "string",
				Got:      item[1].Type(),
			}
		}
		result[key] = val
	}
	return result, nil
}

func starlarkToGo(v starlark.Value) any {
	switch v := v.(type) {
	case starlark.String:
		return string(v)
	case starlark.Int:
		i, _ := v.Int64()
		return i
	case starlark.Float:
		return float64(v)
	case starlark.Bool:
		return bool(v)
	case *starlark.List:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = starlarkToGo(v.Index(i))
		}
		return out
	case starlark.Tuple:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = starlarkToGo(elem)
		}
		return out
	case *starlark.Dict:
		m, _ := starlarkDictToMap(v, "dict")
		return m
	case starlark.NoneType:
		return nil
	default:
		return v.String()
	}
}
