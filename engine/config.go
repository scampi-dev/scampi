package engine

import (
	"fmt"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

type (
	Spec interface {
		Kind() string
		Schema() string
	}
	Config struct{}
)

func loadAndValidate(cfgPath string) (Config, error) {
	ctx := cuecontext.New()
	schema, err := loadUnifiedSchema(ctx)
	if err != nil {
		return Config{}, err
	}

	config, err := loadUserConfig(ctx, cfgPath)
	if err != nil {
		return Config{}, err
	}

	if err := validate(schema, config); err != nil {
		return Config{}, err
	}

	return decodeConfig(config)
}

func loadUnifiedSchema(ctx *cue.Context, specs ...Spec) (cue.Value, error) {
	coreSchema := `
package doit

config: {
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

func decodeConfig(config cue.Value) (Config, error) {
	return Config{}, nil
}
