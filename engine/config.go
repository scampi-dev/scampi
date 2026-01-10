package engine

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
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

type sourceCapturingFS struct {
	cwd   string
	fs    fs.FS
	store *spec.SourceStore
}

func (s sourceCapturingFS) Open(name string) (fs.File, error) {
	f, err := s.fs.Open(name)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	_ = f.Close()

	s.store.AddFile(name, string(data))

	// Give CUE a fresh file
	return s.fs.Open(name)
}

func loadConfig(cfgPath string, store *spec.SourceStore) (spec.Config, error) {
	reg, err := NewRegistry()
	if err != nil {
		return spec.Config{}, err
	}

	ctx := cuecontext.New()

	cwd, err := os.Getwd()
	if err != nil {
		return spec.Config{}, err
	}

	embFS, err := fs.Sub(doit.EmbeddedSchemaModule, "cue")
	if err != nil {
		return spec.Config{}, err
	}

	// One loader config for both schema and user config
	loaderCfg := &load.Config{
		FS: overlayFS{
			Embedded: embFS,
			Host: sourceCapturingFS{
				cwd:   cwd,
				fs:    os.DirFS(cwd),
				store: store,
			},
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

	userFile := userInstances[0].Files[0]

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
			Fields: extractFieldSpansFromFile(userFile, idx),
		}

		cfg.Units = append(cfg.Units, ui)
	}

	return cfg, nil
}

func spanFromPos(pos token.Pos) spec.SourceSpan {
	if !pos.IsValid() {
		return spec.SourceSpan{}
	}

	tf := pos.File()
	if tf == nil {
		return spec.SourceSpan{}
	}

	p := tf.Position(pos)

	return spec.SourceSpan{
		Filename: normalizeVirtualPath(p.Filename),
		Line:     p.Line,
		Column:   p.Column,
	}
}

func extractFieldSpansFromFile(
	f *ast.File,
	unitIndex int,
) map[string]spec.FieldSpan {
	fields := make(map[string]spec.FieldSpan)

	// 1. Find the `units: [...]` field at top level
	var unitsList *ast.ListLit

	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.Field)
		if !ok {
			continue
		}

		label, ok := fd.Label.(*ast.Ident)
		if !ok || label.Name != "units" {
			continue
		}

		list, ok := fd.Value.(*ast.ListLit)
		if !ok {
			return fields
		}

		unitsList = list
		break
	}

	if unitsList == nil {
		return fields
	}

	// 2. Select the unit by index
	if unitIndex < 0 || unitIndex >= len(unitsList.Elts) {
		return fields
	}

	unitExpr := unitsList.Elts[unitIndex]

	// 3. We expect either:
	//    - a struct literal
	//    - or a binary expr like `builtin.copy & { ... }`
	var structLit *ast.StructLit

	switch e := unitExpr.(type) {
	case *ast.StructLit:
		structLit = e

	case *ast.BinaryExpr:
		// match `X & { ... }`
		if rhs, ok := e.Y.(*ast.StructLit); ok {
			structLit = rhs
		}

	default:
		return fields
	}

	if structLit == nil {
		return fields
	}

	// 4. Extract field + value spans from the struct
	for _, elt := range structLit.Elts {
		fd, ok := elt.(*ast.Field)
		if !ok {
			continue
		}

		label, ok := fd.Label.(*ast.Ident)
		if !ok {
			continue
		}

		fields[label.Name] = spec.FieldSpan{
			Field: spanFromPos(label.Pos()),
			Value: spanFromPos(fd.Value.Pos()),
		}
	}

	return fields
}

func normalizeVirtualPath(path string) string {
	parts := strings.Split(path, "/")

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "@fs" {
			continue
		}
		out = append(out, p)
	}

	return strings.Join(out, "/")
}
