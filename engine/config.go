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
	cueerr "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/token"
	"godoit.dev/doit"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
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

func loadConfig(em diagnostic.Emitter, cfgPath string, store *spec.SourceStore) (spec.Config, error) {
	reg := NewRegistry()
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
		var ce cueerr.Error
		if errors.As(err, &ce) {
			return spec.Config{}, CueDiagnostic{
				Err:   ce,
				Phase: "load.userInstances",
			}
		}
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
	var sawAbort bool
	for iter.Next() {
		idx := iter.Selector().Index()
		unitVal := iter.Value()
		unitSpan, fields := extractFieldSpansFromFile(userFile, idx)

		// subject context
		// =============================================
		kind, name, err := resolveUnitIdentity(unitVal, idx)
		if err != nil {
			// FIXME: raw error
			return spec.Config{}, err
		}
		subject := event.Subject{
			Index: idx,
			Kind:  kind,
			Name:  name,
		}

		ut, ok := reg.Type(kind)
		if !ok {
			dr := emitDiagnostics(
				em,
				subject,
				// FIXME: stringy error
				fmt.Errorf("unknown unit kind %q", kind),
			)
			if dr.ShouldAbort() {
				sawAbort = true
			}
			continue
		}

		tCfg := ut.NewConfig()
		// TODO: Check if config is pointer earlier than runtime
		rv := reflect.ValueOf(tCfg)
		if rv.Kind() != reflect.Pointer {
			return spec.Config{}, fmt.Errorf("UnitType['%s'].NewConfig() must return a pointer. Got %T", ut.Kind(), tCfg)
		}

		if err := unitVal.Validate(cue.Concrete(true), cue.All()); err != nil {
			var ce cueerr.Error
			if errors.As(err, &ce) {
				missing := missingRequiredFieldErrors(ce, unitVal, idx, unitSpan, kind, name)
				if len(missing) > 0 {
					for _, m := range missing {
						dr := emitDiagnostics(
							em,
							event.Subject{
								Index: idx,
								Kind:  kind,
								Name:  name,
							},
							MissingFieldDiagnostic{Missing: m},
						)
						if dr.ShouldAbort() {
							sawAbort = true
						}
					}
					continue
				}

				// generic cue error
				dr := emitDiagnostics(
					em,
					subject,
					CueDiagnostic{
						Err:   ce,
						Phase: "decode",
					},
				)
				if dr.ShouldAbort() {
					sawAbort = true
				}
				continue
			}
		}

		if err := unitVal.Decode(tCfg); err != nil {
			var ce cueerr.Error
			if errors.As(err, &ce) {
				return spec.Config{}, CueDiagnostic{
					Err:   ce,
					Phase: "load.decode-config",
				}
			}
		}

		ui := spec.UnitInstance{
			Name:   name,
			Type:   ut,
			Config: tCfg,
			Source: spanFromPos(unitVal.Pos()),
			Fields: fields,
		}

		cfg.Units = append(cfg.Units, ui)
	}

	if sawAbort {
		return spec.Config{}, AbortError{}
	}

	return cfg, nil
}

func resolveUnitIdentity(unitVal cue.Value, idx int) (string, string, error) {
	metaVal := unitVal.LookupPath(cue.ParsePath("meta"))
	if err := metaVal.Err(); err != nil {
		return "", "", err
	}

	kindVal := metaVal.LookupPath(cue.ParsePath("kind"))
	if err := kindVal.Err(); err != nil {
		return "", "", err
	}

	kind, err := kindVal.String()
	if err != nil {
		return "", "", err
	}

	name, err := unitVal.LookupPath(cue.ParsePath("name")).String()
	if err != nil {
		name = fmt.Sprintf("%s[%d]", kind, idx)
	}

	return kind, name, nil
}

func missingRequiredFieldErrors(
	ce cueerr.Error,
	unitVal cue.Value,
	unitIndex int,
	unitSource spec.SourceSpan,
	unitKind string,
	unitName string,
) []CueMissingField {
	var res []CueMissingField

	for _, e := range cueerr.Errors(ce) {
		if !strings.Contains(e.Error(), "incomplete value") {
			continue
		}

		path := cueerr.Path(e)
		if len(path) == 0 {
			continue
		}

		field := path[len(path)-1]

		v := unitVal.LookupPath(cue.ParsePath(field))
		if v.Exists() && !v.IsConcrete() {
			res = append(res, CueMissingField{
				Field:      field,
				UnitIndex:  unitIndex,
				UnitKind:   unitKind,
				UnitSource: unitSource,
				UnitName:   unitName,
			})
		}
	}

	return res
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
		StartCol: p.Column,
		EndCol:   p.Column,
	}
}

func spanFromNode(n ast.Node) spec.SourceSpan {
	if n == nil {
		return spec.SourceSpan{}
	}
	return spanFromPosRange(n.Pos(), n.End())
}

func spanFromPosRange(start, end token.Pos) spec.SourceSpan {
	if !start.IsValid() || !end.IsValid() {
		return spec.SourceSpan{}
	}

	tf := start.File()
	if tf == nil {
		return spec.SourceSpan{}
	}

	sp := tf.Position(start)
	ep := tf.Position(end)

	return spec.SourceSpan{
		Filename: normalizeVirtualPath(sp.Filename),
		Line:     sp.Line,
		StartCol: sp.Column,
		EndCol:   ep.Column,
	}
}

func extractFieldSpansFromFile(
	f *ast.File,
	unitIndex int,
) (spec.SourceSpan, map[string]spec.FieldSpan) {
	// 1. locate `units: [...]`
	var units *ast.ListLit

	for _, d := range f.Decls {
		fd, ok := d.(*ast.Field)
		if !ok {
			continue
		}
		id, ok := fd.Label.(*ast.Ident)
		if !ok || id.Name != "units" {
			continue
		}
		list, ok := fd.Value.(*ast.ListLit)
		if !ok {
			return spec.SourceSpan{}, map[string]spec.FieldSpan{}
		}
		units = list
		break
	}

	if units == nil || unitIndex >= len(units.Elts) {
		return spec.SourceSpan{}, map[string]spec.FieldSpan{}
	}

	// 2. pick the unit expr
	unitExpr := units.Elts[unitIndex]

	// units may be:
	//   { ... }
	//   builtin.copy & { ... }
	var st *ast.StructLit

	switch e := unitExpr.(type) {
	case *ast.StructLit:
		st = e
	case *ast.BinaryExpr:
		if rhs, ok := e.Y.(*ast.StructLit); ok {
			st = rhs
		}
	}

	if st == nil {
		return spec.SourceSpan{}, map[string]spec.FieldSpan{}
	}

	// 3. extract field + value spans
	fields := make(map[string]spec.FieldSpan)
	for _, elt := range st.Elts {
		fd, ok := elt.(*ast.Field)
		if !ok {
			continue
		}

		label, ok := fd.Label.(*ast.Ident)
		if !ok {
			continue
		}

		fields[label.Name] = spec.FieldSpan{
			Field: spanFromNode(label),
			Value: spanFromNode(fd.Value),
		}
	}

	unitSpan := spanFromNode(unitExpr)
	return unitSpan, fields
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
