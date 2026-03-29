// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"fmt"
	"strconv"

	"filippo.io/age"
	"go.starlark.net/starlark"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/spec"
)

func predeclared() starlark.StringDict {
	return starlark.StringDict{
		"apt_repo":  starlark.NewBuiltin("apt_repo", builtinAptRepo),
		"copy":      starlark.NewBuiltin("copy", builtinCopy),
		"dir":       starlark.NewBuiltin("dir", builtinDir),
		"dnf_repo":  starlark.NewBuiltin("dnf_repo", builtinDnfRepo),
		"firewall":  starlark.NewBuiltin("firewall", builtinFirewall),
		"mount":     starlark.NewBuiltin("mount", builtinMount),
		"group":     starlark.NewBuiltin("group", builtinGroup),
		"inline":    starlark.NewBuiltin("inline", builtinInline),
		"local":     starlark.NewBuiltin("local", builtinLocal),
		"pkg":       starlark.NewBuiltin("pkg", builtinPkg),
		"remote":    starlark.NewBuiltin("remote", builtinRemote),
		"run":       starlark.NewBuiltin("run", builtinRun),
		"service":   starlark.NewBuiltin("service", builtinService),
		"system":    starlark.NewBuiltin("system", builtinSystem),
		"sysctl":    starlark.NewBuiltin("sysctl", builtinSysctl),
		"symlink":   starlark.NewBuiltin("symlink", builtinSymlink),
		"template":  starlark.NewBuiltin("template", builtinTemplate),
		"unarchive": starlark.NewBuiltin("unarchive", builtinUnarchive),
		"user":      starlark.NewBuiltin("user", builtinUser),
		"container": containerModule(),
		"rest":      restModule(),
		"target":    targetModule(),
		"deploy":    starlark.NewBuiltin("deploy", builtinDeploy),
		"ref":       starlark.NewBuiltin("ref", builtinRef),
		"env":       starlark.NewBuiltin("env", builtinEnv),
		"secret":    starlark.NewBuiltin("secret", builtinSecret),
		"secrets":   starlark.NewBuiltin("secrets", builtinSecrets),
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
		name     string
		targets  *starlark.List
		steps    *starlark.List
		hooksVal *starlark.Dict
	)
	if err := starlark.UnpackArgs("deploy", args, kwargs,
		"name", &name,
		"targets", &targets,
		"steps", &steps,
		"hooks?", &hooksVal,
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
		return nil, &EmptyNameError{Func: "deploy", Source: s}
	}

	targetNames, err := stringList(targets, "deploy", "targets")
	if err != nil {
		return nil, err
	}
	if len(targetNames) == 0 {
		s := span
		if call != nil {
			if vs, ok := kwargValueSpan(call, "targets"); ok {
				s = vs
			}
		}
		return nil, &EmptyListError{Func: "deploy", Field: "targets", Source: s}
	}

	stepInstances, err := extractSteps(steps, "deploy")
	if err != nil {
		return nil, err
	}
	if len(stepInstances) == 0 {
		s := span
		if call != nil {
			if vs, ok := kwargValueSpan(call, "steps"); ok {
				s = vs
			}
		}
		return nil, &EmptyListError{Func: "deploy", Field: "steps", Source: s}
	}

	hooks, err := extractHooks(hooksVal, span)
	if err != nil {
		return nil, err
	}

	block := spec.DeployBlock{
		Name:    name,
		Targets: targetNames,
		Steps:   stepInstances,
		Hooks:   hooks,
		Source:  span,
	}

	c := threadCollector(thread)
	if err := c.AddDeploy(name, block, span); err != nil {
		return nil, err
	}

	return poisonValue{funcName: "deploy"}, nil
}

func extractHooks(dict *starlark.Dict, span spec.SourceSpan) (map[string][]spec.StepInstance, error) {
	if dict == nil {
		return nil, nil
	}

	hooks := make(map[string][]spec.StepInstance, dict.Len())
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, &TypeError{
				Context:  "deploy: hooks key",
				Expected: "string",
				Got:      item[0].Type(),
				Source:   span,
			}
		}
		if key == "" {
			return nil, &EmptyNameError{Func: "deploy hooks", Source: span}
		}

		steps, err := extractHookSteps(key, item[1], span)
		if err != nil {
			return nil, err
		}
		hooks[key] = steps
	}

	return hooks, nil
}

func extractHookSteps(hookID string, val starlark.Value, span spec.SourceSpan) ([]spec.StepInstance, error) {
	switch v := val.(type) {
	case *StarlarkStep:
		inst := v.Instance
		if inst.Desc == "" && inst.Type != nil {
			inst.Desc = fmt.Sprintf("hook:%s", hookID)
		}
		return []spec.StepInstance{inst}, nil

	case *starlark.List:
		if v.Len() == 0 {
			return nil, &EmptyListError{
				Func:   "deploy hooks",
				Field:  fmt.Sprintf("hooks[%q]", hookID),
				Source: span,
			}
		}
		out := make([]spec.StepInstance, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			step, ok := v.Index(i).(*StarlarkStep)
			if !ok {
				return nil, &TypeError{
					Context:  fmt.Sprintf("deploy: hooks[%q][%d]", hookID, i),
					Expected: "step",
					Got:      v.Index(i).Type(),
					Source:   span,
				}
			}
			inst := step.Instance
			if inst.Desc == "" && inst.Type != nil {
				inst.Desc = fmt.Sprintf("hook:%s[%d]", hookID, i)
			}
			out = append(out, inst)
		}
		return out, nil

	default:
		return nil, &TypeError{
			Context:  fmt.Sprintf("deploy: hooks[%q]", hookID),
			Expected: "step or list of steps",
			Got:      val.Type(),
			Source:   span,
		}
	}
}

func extractSteps(list *starlark.List, fn string) ([]spec.StepInstance, error) {
	if list == nil {
		return nil, nil
	}
	out := make([]spec.StepInstance, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		if err := flattenStep(&out, list.Index(i), fn, i); err != nil {
			return nil, err
		}
	}
	return dedupSteps(out), nil
}

// dedupSteps collapses duplicate steps (same Kind + DedupKey) and remaps
// refs from dropped step IDs to the surviving step's ID.
func dedupSteps(steps []spec.StepInstance) []spec.StepInstance {
	type key struct{ kind, dedup string }

	seen := map[key]spec.StepID{}          // dedup key → survivor ID
	remap := map[spec.StepID]spec.StepID{} // dropped ID → survivor ID
	var out []spec.StepInstance

	for _, s := range steps {
		dk, ok := s.Config.(spec.Deduplicatable)
		if !ok || dk.DedupKey() == "" {
			out = append(out, s)
			continue
		}

		k := key{kind: s.Type.Kind(), dedup: dk.DedupKey()}
		if survivor, exists := seen[k]; exists {
			remap[s.ID] = survivor
			continue
		}

		seen[k] = s.ID
		out = append(out, s)
	}

	if len(remap) > 0 {
		for i := range out {
			remapRefs(out[i].Config, remap)
		}
	}

	return out
}

// remapRefs walks a step config's state dict and replaces ref TargetIDs
// that point to deduped (dropped) steps with the survivor's ID.
func remapRefs(config any, remap map[spec.StepID]spec.StepID) {
	type refMapHolder interface {
		RefMaps() []map[string]any
	}
	rh, ok := config.(refMapHolder)
	if !ok {
		return
	}
	for _, m := range rh.RefMaps() {
		remapRefsInMap(m, remap)
	}
}

func remapRefsInMap(m map[string]any, remap map[spec.StepID]spec.StepID) {
	for k, v := range m {
		switch val := v.(type) {
		case spec.Ref:
			if newID, ok := remap[val.TargetID]; ok {
				val.TargetID = newID
				m[k] = val
			}
		case map[string]any:
			remapRefsInMap(val, remap)
		case []any:
			for i, elem := range val {
				if ref, ok := elem.(spec.Ref); ok {
					if newID, ok := remap[ref.TargetID]; ok {
						ref.TargetID = newID
						val[i] = ref
					}
				}
			}
		}
	}
}

func flattenStep(out *[]spec.StepInstance, v starlark.Value, fn string, idx int) error {
	switch val := v.(type) {
	case *StarlarkStep:
		inst := val.Instance
		if inst.Desc == "" && inst.Type != nil {
			inst.Desc = fmt.Sprintf("%s[%d]", inst.Type.Kind(), idx)
		}
		*out = append(*out, inst)
		return nil
	case *starlark.List:
		for i := 0; i < val.Len(); i++ {
			if err := flattenStep(out, val.Index(i), fn, idx); err != nil {
				return err
			}
		}
		return nil
	case starlark.Tuple:
		for _, elem := range val {
			if err := flattenStep(out, elem, fn, idx); err != nil {
				return err
			}
		}
		return nil
	default:
		return &TypeError{
			Context:  fmt.Sprintf("%s: steps[%d]", fn, idx),
			Expected: "step or list of steps",
			Got:      v.Type(),
		}
	}
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
			Source: firstArgSpan(thread),
		}
	}

	c := threadCollector(thread)
	envVal, found := c.src.LookupEnv(key)

	// No default → required
	if len(args) == 1 {
		if !found {
			return nil, &EnvVarRequiredError{
				Key:    key,
				Source: firstArgSpan(thread),
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

// secret(key)
// -----------------------------------------------------------------------------

func builtinSecret(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	span := callSpan(thread)

	if len(args) != 1 {
		return nil, &SecretError{
			Detail: fmt.Sprintf("accepts exactly 1 positional argument, got %d", len(args)),
			Source: span,
		}
	}
	if len(kwargs) > 0 {
		return nil, &SecretError{
			Detail: "does not accept keyword arguments",
			Source: span,
		}
	}

	key, ok := starlark.AsString(args[0])
	if !ok {
		return nil, &SecretError{
			Detail: fmt.Sprintf("key must be a string, got %s", args[0].Type()),
			Source: firstArgSpan(thread),
		}
	}

	c := threadCollector(thread)
	if !c.secretsConfigured {
		return nil, &SecretsConfigError{
			Detail: `secret() requires a secrets() backend; add e.g.` +
				` secrets(backend="age", path="secrets.age.json") before any secret() call`,
			Source: span,
		}
	}
	val, found, err := c.src.LookupSecret(key)
	if err != nil {
		return nil, &SecretBackendError{
			Key:    key,
			Cause:  err,
			Source: span,
		}
	}
	if !found {
		return nil, &SecretNotFoundError{
			Key:    key,
			Source: firstArgSpan(thread),
		}
	}

	return starlark.String(val), nil
}

// secrets(backend, path, recipients?)
// -----------------------------------------------------------------------------

func builtinSecrets(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	span := callSpan(thread)

	if len(args) > 0 {
		return nil, &SecretsConfigError{
			Detail: "accepts only keyword arguments",
			Source: span,
		}
	}

	var backend, path string
	var recipientsVal starlark.Value
	if err := starlark.UnpackArgs("secrets", args, kwargs,
		"backend", &backend,
		"path?", &path,
		"recipients?", &recipientsVal,
	); err != nil {
		if backend == "" {
			backend = "age"
		}
		if path == "" {
			path = "secrets." + backend + ".json"
		}
		return nil, &SecretsConfigError{
			Detail: fmt.Sprintf(
				`%s; e.g. secrets(backend=%q, path=%q)`,
				err.Error(), backend, path,
			),
			Source: span,
		}
	}

	switch backend {
	case "file", "age":
	default:
		s := span
		pos := callerPosition(thread)
		if call := findCallFromThread(thread, pos); call != nil {
			if vs, ok := kwargValueSpan(call, "backend"); ok {
				s = vs
			}
		}
		return nil, &SecretsConfigError{
			Detail: fmt.Sprintf("unknown backend %q (available: file, age)", backend),
			Source: s,
		}
	}

	if path == "" {
		path = "secrets." + backend + ".json"
	}

	c := threadCollector(thread)

	b, err := buildSecretBackend(c, backend, path, recipientsVal, span)
	if err != nil {
		return nil, err
	}

	if !c.SetSecretBackend(b) {
		return nil, &SecretsConfigError{
			Detail: "secrets() can only be called once per config",
			Source: span,
		}
	}

	return poisonValue{funcName: "secrets"}, nil
}

func buildSecretBackend(
	c *Collector,
	backend, path string,
	recipientsVal starlark.Value,
	span spec.SourceSpan,
) (secret.Backend, error) {
	switch backend {
	case "file":
		data, err := c.src.ReadFile(c.ctx, path)
		if err != nil {
			return nil, &SecretsConfigError{
				Detail: fmt.Sprintf("reading secrets file %q: %s", path, err),
				Source: span,
			}
		}
		b, err := secret.NewFileBackend(data)
		if err != nil {
			return nil, &SecretsConfigError{
				Detail: fmt.Sprintf("parsing secrets file %q: %s", path, err),
				Source: span,
			}
		}
		return b, nil

	case "age":
		_, err := parseRecipientStrings(recipientsVal, span)
		if err != nil {
			return nil, err
		}

		readFile := func(path string) ([]byte, error) {
			return c.src.ReadFile(c.ctx, path)
		}
		identities, err := secret.ResolveIdentities(
			c.src.LookupEnv,
			readFile,
		)
		if err != nil {
			return nil, &SecretsConfigError{
				Detail: err.Error(),
				Source: span,
			}
		}

		data, err := c.src.ReadFile(c.ctx, path)
		if err != nil {
			return nil, &SecretsConfigError{
				Detail: fmt.Sprintf("reading secrets file %q: %s", path, err),
				Source: span,
			}
		}

		b, err := secret.NewAgeBackend(data, identities)
		if err != nil {
			return nil, &SecretsConfigError{
				Detail: fmt.Sprintf("decrypting secrets file %q: %s", path, err),
				Source: span,
			}
		}
		return b, nil

	}

	panic(errs.BUG("unreachable: backend validated before buildSecretBackend"))
}

// parseRecipientStrings extracts age recipient public keys from a Starlark list value.
func parseRecipientStrings(val starlark.Value, span spec.SourceSpan) ([]age.Recipient, error) {
	if val == nil || val == starlark.None {
		return nil, nil
	}

	list, ok := val.(*starlark.List)
	if !ok {
		return nil, &SecretsConfigError{
			Detail: fmt.Sprintf("recipients must be a list, got %s", val.Type()),
			Source: span,
		}
	}

	recipients := make([]age.Recipient, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		s, ok := starlark.AsString(list.Index(i))
		if !ok {
			return nil, &SecretsConfigError{
				Detail: fmt.Sprintf("recipients[%d] must be a string, got %s", i, list.Index(i).Type()),
				Source: span,
			}
		}
		r, err := age.ParseX25519Recipient(s)
		if err != nil {
			return nil, &SecretsConfigError{
				Detail: fmt.Sprintf("recipients[%d]: invalid age recipient %q: %s", i, s, err),
				Source: span,
			}
		}
		recipients = append(recipients, r)
	}
	return recipients, nil
}

func coerceEnvValue(
	raw string,
	dflt starlark.Value,
	span spec.SourceSpan,
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
