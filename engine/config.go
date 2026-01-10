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
	"cuelang.org/go/cue/token"
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

	unitsVal := cfgVal.LookupPath(cue.ParsePath("units"))
	if err := unitsVal.Err(); err != nil {
		return spec.Config{}, err
	}

	iter, err := unitsVal.List()
	if err != nil {
		return spec.Config{}, err
	}

	cfg := spec.Config{}
	for iter.Next() {
		idx := iter.Selector().Index()
		unitVal := iter.Value()

		metaVal := unitVal.LookupPath(cue.ParsePath("meta"))
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

		name, err := unitVal.LookupPath(cue.ParsePath("name")).String()
		if err != nil {
			name = fmt.Sprintf("%s[%d]", kind, idx)
		}

		typ, ok := reg.Type(kind)
		if !ok {
			return spec.Config{}, fmt.Errorf("unknown unit kind %q", kind)
		}

		tCfg := typ.NewConfig()
		// TODO: Check if config is pointer earlier than runtime
		rv := reflect.ValueOf(tCfg)
		if rv.Kind() != reflect.Pointer {
			return spec.Config{}, fmt.Errorf("UnitType['%s'].NewConfig() must return a pointer. Got %T", typ.Kind(), tCfg)
		}

		if err := unitVal.Decode(tCfg); err != nil {
			return spec.Config{}, err
		}

		ui := spec.UnitInstance{
			Name:   name,
			Type:   typ,
			Config: tCfg,
			Source: spanFromPos(unitVal.Pos()),
			Fields: map[string]spec.SourceSpan{},
		}

		it, _ := unitVal.Fields()
		for it.Next() {
			ui.Fields[it.Label()] = spanFromPos(it.Value().Pos())
		}

		cfg.Units = append(cfg.Units, ui)
	}

	return cfg, nil
}

func spanFromPos(pos token.Pos) spec.SourceSpan {
	if pos == token.NoPos {
		return spec.SourceSpan{}
	}

	tf := pos.File()
	if tf == nil {
		return spec.SourceSpan{}
	}

	p := tf.Position(pos)

	return spec.SourceSpan{
		Filename: p.Filename,
		Line:     p.Line,
		Column:   p.Column,
	}
}
