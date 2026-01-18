package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	cueerr "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/token"
	"godoit.dev/doit"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/diagnostic/event"
	"godoit.dev/doit/source"
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
		return o.Host.Open(name)
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

type memFile struct {
	name string
	data []byte
	pos  int
}

func newMemFile(name string, data []byte) *memFile {
	// defensive copy so callers can't mutate through the slice
	cp := make([]byte, len(data))
	copy(cp, data)

	return &memFile{
		name: name,
		data: cp,
	}
}

func (f *memFile) Read(p []byte) (int, error) {
	if f.pos >= len(f.data) {
		return 0, io.EOF
	}

	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}

func (f *memFile) Close() error {
	return nil
}

func (f *memFile) Stat() (fs.FileInfo, error) {
	return memFileInfo{
		name: f.name,
		size: int64(len(f.data)),
	}, nil
}

type memFileInfo struct {
	name string
	size int64
}

func (i memFileInfo) Name() string       { return path.Base(i.name) }
func (i memFileInfo) Size() int64        { return i.size }
func (i memFileInfo) Mode() fs.FileMode  { return 0o644 }
func (i memFileInfo) ModTime() time.Time { return time.Time{} }
func (i memFileInfo) IsDir() bool        { return false }
func (i memFileInfo) Sys() any           { return nil }

type sourceFS struct {
	src source.Source
}

func (s sourceFS) Open(name string) (fs.File, error) {
	if strings.HasPrefix(name, "/") {
		return nil, fmt.Errorf("BUG: fs.FS received absolute path %q", name)
	}

	p := "/" + name
	data, err := s.src.ReadFile(context.Background(), p)
	if err != nil {
		return nil, err
	}

	return newMemFile(name, data), nil
}

// LoadConfig decodes and validates user configuration.
// It returns ONLY user-facing configuration errors.
// All other failures are engine or environment bugs and will panic.
func LoadConfig(em diagnostic.Emitter, cfgPath string, store *spec.SourceStore) (spec.Config, error) {
	return LoadConfigWithSource(
		em,
		cfgPath,
		store,
		source.LocalPosixSource{},
	)
}

func LoadConfigWithSource(
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	src source.Source,
) (spec.Config, error) {
	cfg, err := loadConfigWithSource(em, cfgPath, store, src)
	if err != nil {
		dr, _ := emitDiagnostics(
			em,
			event.Subject{
				CfgPath: cfgPath,
			},
			err,
		)
		if dr.ShouldAbort() {
			return spec.Config{}, AbortError{Causes: []error{err}}
		}
		return spec.Config{}, panicIfNotAbortError(err)
	}

	return cfg, nil
}

func loadConfigWithSource(
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	src source.Source,
) (spec.Config, error) {
	reg := NewRegistry()
	ctx := cuecontext.New()

	embFS, err := fs.Sub(doit.EmbeddedSchemaModule, "cue")
	if err != nil {
		panic(fmt.Errorf("BUG: embedded schema FS corrupted: %w", err))
	}

	// One loader config for both schema and user config
	loaderCfg := &load.Config{
		FS: overlayFS{
			Embedded: embFS,
			Host: sourceCapturingFS{
				fs: sourceFS{
					src: src,
				},
				store: store,
			},
		},
		Dir: ".",
	}

	userInstances := load.Instances([]string{cfgPath}, loaderCfg)
	if len(userInstances) == 0 {
		panic("BUG: load.Instances returned zero instances")
	}
	if err := userInstances[0].Err; err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(fmt.Errorf(
				"BUG: load.Instances returned an unexpected error for cfgPath %q: %w",
				cfgPath,
				err,
			))
		}

		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.userInstances",
		}
	}

	userInst := ctx.BuildInstance(userInstances[0])
	if err := userInst.Err(); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(fmt.Errorf(
				"BUG: load.BuildInstance returned an unexpected error for cfgPath %q: %w",
				cfgPath,
				err,
			))
		}

		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.BuildInstance",
		}
	}

	userFile := userInstances[0].Files[0]

	coreInstances := load.Instances([]string{"godoit.dev/doit/core"}, loaderCfg)
	if len(coreInstances) == 0 {
		panic("BUG: load.Instances returned zero core-instances")
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
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			// unexpected validation failure → engine error
			panic(fmt.Errorf(
				"BUG: coreInst.Value().Unify(userInst) failed with an unaccounted-for error-type %T: %w",
				err,
				err,
			))
		}

		if isUnitsShapeError(ce) {
			return spec.Config{}, InvalidUnitsShape{
				Source: getUnifyErrorSpan(ce, userInst),
				Have:   describeCueValueShape(userInst, "units"),
				Want:   describeCueValueShape(coreInst, "units"),
			}
		}

		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.unifyCoreAndUserInst",
		}
	}

	unitsVal := cfgVal.LookupPath(cue.ParsePath("units"))
	if err := unitsVal.Err(); err != nil {
		return spec.Config{}, err
	}

	iter, err := unitsVal.List()
	if err != nil {
		panic(fmt.Errorf("BUG: units is not a list after schema unification: %w", err))
	}

	cfg := spec.Config{}
	var sawAbort bool
	for iter.Next() {
		idx := iter.Selector().Index()
		unitVal := iter.Value()
		unitSpan, fields := extractFieldSpansFromFile(userFile, idx)

		ui, decRes := decodeUnit(unitVal, idx, reg, em, unitSpan, fields)
		if decRes.abort {
			sawAbort = true
		}

		if !decRes.ok {
			continue
		}

		cfg.Units = append(cfg.Units, ui)
	}

	if sawAbort {
		return spec.Config{}, AbortError{}
	}

	return cfg, nil
}

type decodeResult struct {
	abort bool
	ok    bool
}

func decodeUnit(
	unitVal cue.Value,
	unitIdx int,
	reg *Registry,
	em diagnostic.Emitter,
	unitSpan spec.SourceSpan,
	fields map[string]spec.FieldSpan,
) (spec.UnitInstance, decodeResult) {
	// ------------------------------------------------------------
	// Resolve identity (non-diagnostic for now)
	// ------------------------------------------------------------
	kind, name, err := resolveUnitIdentity(unitVal, unitIdx)
	if err != nil {
		// engine/schema error – cannot continue safely
		panic(fmt.Errorf(
			"BUG: resolveUnitIdentity returned an unexpected error for unit %q (%s): %w",
			name,
			kind,
			err,
		))
	}

	subject := event.Subject{
		Index: unitIdx,
		Kind:  kind,
		Name:  name,
	}

	// ------------------------------------------------------------
	// Resolve unit type
	// ------------------------------------------------------------
	ut, ok := reg.Type(kind)
	if !ok {
		dr, _ := emitDiagnostics(
			em,
			subject,
			fmt.Errorf("unknown unit kind %q", kind),
		)
		return spec.UnitInstance{}, decodeResult{
			abort: dr.ShouldAbort(),
			ok:    false,
		}
	}

	// ------------------------------------------------------------
	// Instantiate config
	// ------------------------------------------------------------
	tCfg := ut.NewConfig()
	rv := reflect.ValueOf(tCfg)
	if rv.Kind() != reflect.Pointer {
		// internal error
		panic(fmt.Errorf(
			"UnitType['%s'].NewConfig() must return a pointer (got %T)",
			ut.Kind(),
			tCfg,
		))
	}

	// ------------------------------------------------------------
	// VALIDATION PHASE (user fault, rich diagnostics)
	// ------------------------------------------------------------
	if err := unitVal.Validate(cue.Concrete(true), cue.All()); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			// unexpected validation failure → engine error
			panic(fmt.Errorf(
				"BUG: unitVal.Validate failed with an unaccounted-for error-type %T for unit %q (%s): %w",
				err,
				name,
				kind,
				err,
			))
		}

		missing := missingRequiredFieldErrors(
			ce,
			unitVal,
			unitIdx,
			unitSpan,
			kind,
			name,
		)

		if len(missing) > 0 {
			var abort bool
			for _, m := range missing {
				dr, _ := emitDiagnostics(
					em,
					subject,
					MissingFieldDiagnostic{Missing: m},
				)
				if dr.ShouldAbort() {
					abort = true
				}
			}
			return spec.UnitInstance{}, decodeResult{
				abort: abort,
				ok:    false,
			}
		}

		// generic cue validation error, still user-facing
		dr, _ := emitDiagnostics(
			em,
			subject,
			CueDiagnostic{
				Err:   ce,
				Phase: "decode",
			},
		)

		return spec.UnitInstance{}, decodeResult{
			abort: dr.ShouldAbort(),
			ok:    false,
		}
	}

	// ------------------------------------------------------------
	// DECODE PHASE (engine/schema invariant)
	// ------------------------------------------------------------
	if err := unitVal.Decode(tCfg); err != nil {
		// If Validate passed, Decode MUST NOT fail.
		// This is a hard invariant violation.
		panic(fmt.Errorf(
			"BUG: unitVal.Decode failed after successful validation for unit %q (%s): %w",
			name,
			kind,
			err,
		))
	}

	// ------------------------------------------------------------
	// Success
	// ------------------------------------------------------------
	ui := spec.UnitInstance{
		Name:   name,
		Type:   ut,
		Config: tCfg,
		Source: spanFromPos(unitVal.Pos()),
		Fields: fields,
	}

	return ui, decodeResult{
		abort: false,
		ok:    true,
	}
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

func isUnitsShapeError(ce cueerr.Error) bool {
	for _, e := range cueerr.Errors(ce) {
		path := cueerr.Path(e)
		if len(path) == 1 && path[0] == "units" {
			return true
		}
	}
	return false
}

func describeCueValueShape(base cue.Value, path string) string {
	v := base.LookupPath(cue.ParsePath(path))
	switch v.Kind() {
	case cue.ListKind:
		return "list"
	case cue.StructKind:
		return "struct"
	default:
		return v.Kind().String()
	}
}

func getUnifyErrorSpan(err cueerr.Error, userInst cue.Value) spec.SourceSpan {
	path := cueerr.Path(err)

	// Case 1: error is inside units[<idx>]
	if len(path) >= 2 && path[0] == "units" {
		if idx, err := strconv.Atoi(path[1]); err == nil {
			v := userInst.LookupPath(
				cue.MakePath(cue.Str("units"), cue.Index(idx)),
			)
			return spanFromPos(v.Pos())
		}
	}

	// Case 2: error is about top-level structure (e.g. units itself)
	if len(path) > 0 {
		v := userInst.LookupPath(cue.MakePath(cue.Str(path[0])))
		return spanFromPos(v.Pos())
	}

	// Case 3: fallback → start-of-file
	return spanFromPos(userInst.Pos())
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

	return "/" + strings.Join(out, "/")
}
