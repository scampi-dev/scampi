package engine

import (
	"fmt"
	"path/filepath"
	"reflect"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"godoit.dev/doit/spec"
)

func loadAndValidate(cfgPath string, reg *Registry) (spec.Config, error) {
	ctx := cuecontext.New()

	schema, err := loadUnifiedSchema(ctx, reg.Specs())
	if err != nil {
		return spec.Config{}, err
	}

	config, err := loadUserConfig(ctx, cfgPath)
	if err != nil {
		return spec.Config{}, err
	}

	if err := validate(schema, config); err != nil {
		return spec.Config{}, err
	}

	return decodeConfig(config, reg)
}

func loadUnifiedSchema(ctx *cue.Context, specs []spec.Spec) (cue.Value, error) {
	coreSchema := `
package doit

playbook: {
	tasks: [...#Task]
}

#Task: {
	kind: string
	...
}
`
	val := ctx.CompileString(coreSchema)
	if err := val.Err(); err != nil {
		return cue.Value{}, err
	}

	for _, s := range specs {
		specVal := ctx.CompileString(s.Schema())
		if err := specVal.Err(); err != nil {
			return cue.Value{}, fmt.Errorf("spec[%q] - schema error: %w", s.Kind(), err)
		}

		val = val.Unify(specVal)

		if err := val.Err(); err != nil {
			return cue.Value{}, err
		}
	}

	if err := val.Err(); err != nil {
		return cue.Value{}, err
	}

	return val, nil
}

func loadUserConfig(ctx *cue.Context, cfgPath string) (cue.Value, error) {
	if filepath.Ext(cfgPath) != ".cue" {
		return cue.Value{}, fmt.Errorf("only .cue configs are supported")
	}

	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		return cue.Value{}, err
	}

	inst := load.Instances(
		[]string{filepath.Base(abs)},
		&load.Config{Dir: filepath.Dir(abs)},
	)

	if len(inst) != 1 {
		return cue.Value{}, fmt.Errorf("expected exactly one CUE instance")
	}

	val := ctx.BuildInstance(inst[0])
	if err := val.Err(); err != nil {
		return cue.Value{}, err
	}

	return val, nil
}

func validate(schema, config cue.Value) error {
	val := schema.Unify(config)
	if err := val.Validate(cue.Concrete(true)); err != nil {
		return err
	}

	return nil
}

func decodeConfig(configVal cue.Value, reg *Registry) (spec.Config, error) {
	tasksVal := configVal.LookupPath(cue.ParsePath("playbook.tasks"))
	if err := tasksVal.Err(); err != nil {
		return spec.Config{}, err
	}

	iter, err := tasksVal.List()
	if err != nil {
		return spec.Config{}, err
	}

	cfg := spec.Config{}
	for iter.Next() {
		taskVal := iter.Value()

		var kind string
		if err := taskVal.LookupPath(cue.ParsePath("kind")).Decode(&kind); err != nil {
			return spec.Config{}, err
		}

		s, ok := reg.SpecForKind(kind)
		if !ok {
			return spec.Config{}, fmt.Errorf("task kind %s not found in schema", kind)
		}

		out := s.NewConfig()

		if !isPointer(out) {
			return spec.Config{}, fmt.Errorf("spec['%s'].NewConfig must return a pointer. Got %T", kind, out)
		}

		if err := decodeTask(taskVal, kind, out); err != nil {
			return spec.Config{}, err
		}

		cfg.Tasks = append(cfg.Tasks, spec.CfgTask{
			Kind:   kind,
			Spec:   s,
			Config: out,
		})
	}

	return cfg, nil
}

func decodeTask(taskVal cue.Value, kind string, out any) error {
	specVal := taskVal.LookupPath(cue.ParsePath(kind))
	if !specVal.Exists() {
		return fmt.Errorf("task of kind %q is missing required %q block", kind, kind)
	}

	if err := specVal.Decode(out); err != nil {
		return fmt.Errorf("failed to decode %q task config: %w", kind, err)
	}

	return nil
}

func isPointer(i any) bool {
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Pointer
}
