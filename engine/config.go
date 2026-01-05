package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"godoit.dev/doit"
	"godoit.dev/doit/spec"
)

type overlayFS struct {
	Embedded fs.FS
	Host     fs.FS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	f, err := o.Embedded.Open(name)
	if err == nil {
		return f, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		s, err := o.Host.Open(name)
		if err == nil {
			return s, err
		}

		return s, err
	}

	return nil, err
}

func loadConfig(cfgPath string) (spec.Config, error) {
	reg, err := NewRegistry()
	if err != nil {
		return spec.Config{}, err
	}

	ctx := cuecontext.New()

	cwd, err := os.Getwd()
	if err != nil {
		return spec.Config{}, err
	}

	emb, err := fs.Sub(doit.EmbeddedSchemaModule, "cue")
	if err != nil {
		return spec.Config{}, err
	}

	// One loader config for both schema and user config
	loaderCfg := &load.Config{
		FS: overlayFS{
			Embedded: emb,
			Host:     os.DirFS(cwd),
		},
		Dir: ".",
	}

	// user config
	userInstances := load.Instances([]string{cfgPath}, loaderCfg)
	if len(userInstances) == 0 {
		return spec.Config{}, fmt.Errorf("no user instances loaded")
	}
	if err := userInstances[0].Err; err != nil {
		return spec.Config{}, err
	}

	userInst := ctx.BuildInstance(userInstances[0])
	if err := userInst.Err(); err != nil {
		return spec.Config{}, err
	}

	coreInstances := load.Instances([]string{"godoit.dev/doit/core"}, loaderCfg)
	if len(coreInstances) == 0 {
		return spec.Config{}, fmt.Errorf("no core instances loaded")
	}
	if err := coreInstances[0].Err; err != nil {
		return spec.Config{}, err
	}

	coreInst := ctx.BuildInstance(coreInstances[0])
	if err := coreInst.Err(); err != nil {
		return spec.Config{}, err
	}

	// --- apply schema ---
	cfgVal := coreInst.Value().Unify(userInst)
	if err := cfgVal.Err(); err != nil {
		return spec.Config{}, err
	}

	tasksVal := cfgVal.LookupPath(cue.ParsePath("tasks"))
	if err := tasksVal.Err(); err != nil {
		return spec.Config{}, err
	}

	iter, err := tasksVal.List()
	if err != nil {
		return spec.Config{}, err
	}

	cfg := spec.Config{}
	for iter.Next() {
		idx := iter.Selector().Index()
		taskVal := iter.Value()

		metaVal := taskVal.LookupPath(cue.ParsePath("meta"))
		if err := metaVal.Err(); err != nil {
			return spec.Config{}, err
		}

		kindVal := metaVal.LookupPath(cue.ParsePath("kind"))
		if err := kindVal.Err(); err != nil {
			return spec.Config{}, err
		}

		kind, err := kindVal.String()
		if err != nil {
			return spec.Config{}, err
		}

		name, err := taskVal.LookupPath(cue.ParsePath("name")).String()
		if err != nil {
			name = fmt.Sprintf("%s[%d]", kind, idx)
		}

		s, ok := reg.SpecForKind(kind)
		if !ok {
			return spec.Config{}, fmt.Errorf("unknown task kind %q", kind)
		}

		c := s.NewConfig()
		// TODO: Check if config is pointer earlier than runtime
		rv := reflect.ValueOf(c)
		if rv.Kind() != reflect.Pointer {
			return spec.Config{}, fmt.Errorf("spec['%s'].NewConfig must return a pointer. Got %T", s.Kind(), c)
		}

		if err := taskVal.Decode(c); err != nil {
			return spec.Config{}, err
		}

		cfg.Tasks = append(cfg.Tasks, spec.CfgTask{
			Name:   name,
			Spec:   s,
			Config: c,
		})
	}

	return cfg, nil
}
