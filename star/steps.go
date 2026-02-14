// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"fmt"

	"go.starlark.net/starlark"

	"godoit.dev/doit/spec"
	stepcopy "godoit.dev/doit/step/copy"
	"godoit.dev/doit/step/dir"
	"godoit.dev/doit/step/pkg"
	"godoit.dev/doit/step/symlink"
	"godoit.dev/doit/step/template"
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
		src   string
		dest  string
		perm  string
		owner string
		group string
		desc  string
	)
	if err := starlark.UnpackArgs("copy", args, kwargs,
		"src", &src,
		"dest", &dest,
		"perm", &perm,
		"owner", &owner,
		"group", &group,
		"desc?", &desc,
	); err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc:   desc,
			Type:   stepcopy.Copy{},
			Config: &stepcopy.CopyConfig{Desc: desc, Src: src, Dest: dest, Perm: perm, Owner: owner, Group: group},
			Source: span,
			Fields: kwargsFieldSpans(thread, "src", "dest", "perm", "owner", "group"),
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
		path  string
		perm  string
		owner string
		group string
		desc  string
	)
	if err := starlark.UnpackArgs("dir", args, kwargs,
		"path", &path,
		"perm?", &perm,
		"owner?", &owner,
		"group?", &group,
		"desc?", &desc,
	); err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc:   desc,
			Type:   dir.Dir{},
			Config: &dir.DirConfig{Desc: desc, Path: path, Perm: perm, Owner: owner, Group: group},
			Source: span,
			Fields: kwargsFieldSpans(thread, "path", "perm", "owner", "group"),
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
		packages *starlark.List
		state    = "present"
		desc     string
	)
	if err := starlark.UnpackArgs("pkg", args, kwargs,
		"packages", &packages,
		"state?", &state,
		"desc?", &desc,
	); err != nil {
		return nil, err
	}

	pkgs, err := stringList(packages, "pkg", "packages")
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc:   desc,
			Type:   pkg.Pkg{},
			Config: &pkg.PkgConfig{Desc: desc, Packages: pkgs, State: state},
			Source: span,
			Fields: kwargsFieldSpans(thread, "packages", "state"),
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
		target string
		link   string
		desc   string
	)
	if err := starlark.UnpackArgs("symlink", args, kwargs,
		"target", &target,
		"link", &link,
		"desc?", &desc,
	); err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc:   desc,
			Type:   symlink.Symlink{},
			Config: &symlink.SymlinkConfig{Desc: desc, Target: target, Link: link},
			Source: span,
			Fields: kwargsFieldSpans(thread, "target", "link"),
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
		dest    string
		perm    string
		owner   string
		group   string
		src     string
		content string
		data    *starlark.Dict
		desc    string
	)
	if err := starlark.UnpackArgs("template", args, kwargs,
		"dest", &dest,
		"perm", &perm,
		"owner", &owner,
		"group", &group,
		"src?", &src,
		"content?", &content,
		"data?", &data,
		"desc?", &desc,
	); err != nil {
		return nil, err
	}

	dataCfg, err := convertDataConfig(data)
	if err != nil {
		return nil, err
	}

	span := callSpan(thread)
	return &StarlarkStep{
		Instance: spec.StepInstance{
			Desc: desc,
			Type: template.Template{},
			Config: &template.TemplateConfig{
				Desc: desc, Src: src, Content: content, Dest: dest,
				Data: dataCfg, Perm: perm, Owner: owner, Group: group,
			},
			Source: span,
			Fields: kwargsFieldSpans(thread,
				"dest", "perm", "owner", "group", "src", "content"),
		},
	}, nil
}

// Helpers
// -----------------------------------------------------------------------------

func stringList(
	list *starlark.List, fn, arg string,
) ([]string, error) {
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
		result[key] = starlarkToGo(item[1])
	}
	return result, nil
}

func starlarkDictToStringMap(
	dict *starlark.Dict, ctx string,
) (map[string]string, error) {
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
