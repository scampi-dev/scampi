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
	"sync"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	cueerr "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/token"
	"github.com/cespare/xxhash/v2"
	"godoit.dev/doit"
	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/errs"
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
	fs          fs.FS
	validSource sync.Map // hash -> error (nil OK)
	store       *spec.SourceStore
}

func (s *sourceCapturingFS) Open(name string) (fs.File, error) {
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

	hash := xxhash.Sum64(data)
	if v, ok := s.validSource.Load(hash); ok {
		if v != nil {
			if err := v.(error); err != nil {
				return nil, err
			}
		}
	} else {
		// Reject inputs that would cause CUE to hang or exhaust resources.
		err := ValidateCueInput(data)
		s.validSource.Store(hash, err)
		s.store.AddFile(name, string(data))
		if err != nil {
			return nil, err
		}
	}

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
	ctx context.Context
	src source.Source
}

func (s sourceFS) Open(name string) (fs.File, error) {
	if strings.HasPrefix(name, "/") {
		return nil, errs.BUG("fs.FS received absolute path %q", name)
	}

	p := "/" + name
	data, err := s.src.ReadFile(s.ctx, p)
	if err != nil {
		return nil, err
	}

	return newMemFile(name, data), nil
}

const (
	cueUnit   = "unit"
	cueUnitID = "id"
	cueSteps  = "steps"
)

// LoadConfig decodes and validates user configuration.
// It returns ONLY user-facing configuration errors.
// All other failures are engine or environment bugs and will panic.
func LoadConfig(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
) (spec.Config, error) {
	return LoadConfigWithSource(
		ctx,
		em,
		cfgPath,
		store,
		source.LocalPosixSource{},
	)
}

func LoadConfigWithSource(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	src source.Source,
) (cfg spec.Config, err error) {
	cfg, err = loadConfigWithSource(ctx, em, cfgPath, store, src)
	if err != nil {
		impact, _ := emitEngineDiagnostic(em, cfgPath, err)
		if impact.Is(diagnostic.ImpactAbort) {
			return spec.Config{}, AbortError{Causes: []error{err}}
		}
		return spec.Config{}, panicIfNotAbortError(err)
	}

	return cfg, nil
}

func loadConfigWithSource(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	src source.Source,
) (cfg spec.Config, err error) {
	// Guard against panics in the CUE library (known upstream bugs).
	// Convert to a user-facing diagnostic rather than crashing.
	defer func() {
		if r := recover(); r != nil {
			panicErr := CuePanic{Recovered: r}
			_, _ = emitEngineDiagnostic(em, cfgPath, panicErr)
			err = AbortError{Causes: []error{panicErr}}
		}
	}()

	cfg, err = loadConfigWithSourceUnsafe(ctx, em, cfgPath, store, src)
	return
}

func loadConfigWithSourceUnsafe(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	src source.Source,
) (spec.Config, error) {
	reg := NewRegistry()
	cueCtx := cuecontext.New()

	embFS, err := fs.Sub(doit.EmbeddedSchemaModule, "cue")
	if err != nil {
		panic(errs.BUG("embedded schema FS corrupted: %w", err))
	}

	// One loader config for both schema and user config
	loaderCfg := &load.Config{
		FS: overlayFS{
			Embedded: embFS,
			Host: &sourceCapturingFS{
				fs: sourceFS{
					ctx: ctx,
					src: src,
				},
				store: store,
			},
		},
		Dir: ".",
	}

	userInstances := load.Instances([]string{cfgPath}, loaderCfg)
	if len(userInstances) == 0 {
		panic(errs.BUG("load.Instances returned zero instances for '%s'", cfgPath))
	}
	userInstance := userInstances[0]
	if err := userInstance.Err; err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.Instances returned an unexpected error for cfgPath %q: %w",
				cfgPath,
				err,
			))
		}

		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.userInstances",
		}
	}

	userInst := cueCtx.BuildInstance(userInstance)
	if err := userInst.Err(); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.BuildUserInstance returned an unexpected error for cfgPath %q: %w",
				cfgPath,
				err,
			))
		}

		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.BuildUserInstance",
		}
	}

	coreInstances := load.Instances([]string{"godoit.dev/doit/core"}, loaderCfg)
	if len(coreInstances) == 0 {
		panic(errs.BUG("load.Instances returned zero core-instances"))
	}
	if err := coreInstances[0].Err; err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.CoreInstances returned an unexpected error for cfgPath %q: %w",
				cfgPath,
				err,
			))
		}

		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.CoreInstances",
		}
	}

	coreInst := cueCtx.BuildInstance(coreInstances[0])
	if err := coreInst.Err(); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.BuildCoreInstance returned an unexpected error for cfgPath %q: %w",
				cfgPath,
				err,
			))
		}

		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.BuildCoreInstance",
		}
	}

	// --- apply schema ---
	cfgVal := coreInst.Value().Unify(userInst)
	if err := cfgVal.Err(); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			// unexpected validation failure → engine error
			panic(errs.BUG(
				"load.Unify failed with an unaccounted-for error-type %T: %w",
				err,
				err,
			))
		}

		if isErrorWithPath(ce, cueSteps) {
			return spec.Config{}, InvalidStepsShape{
				Source: getStepsUnifyErrorSpan(ce, userInst),
				Have:   describeCueValueShape(userInst, cueSteps),
				Want:   describeCueSchemaShape(coreInst, cueSteps),
			}
		}
		if isErrorWithPath(ce, cueUnit) {
			return spec.Config{}, InvalidUnitShape{
				Source: getStepsUnifyErrorSpan(ce, userInst),
				Have:   describeCueValueShape(userInst, cueUnit),
				Want:   describeCueSchemaShape(coreInst, cueUnit),
			}
		}
		if path, ok := isTypeMismatchError(ce); ok {
			return spec.Config{}, TypeMismatch{
				Source: getErrorPathSpan(ce, userInst),
				Path:   path,
				Have:   describeCueValueShape(userInst, path),
				Want:   describeCueSchemaShape(coreInst, path),
			}
		}

		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.Unify",
		}
	}

	userFile := userInstance.Files[0]
	unitInst, err := decodeUnit(cfgVal, cfgPath, userFile, em)
	if err != nil {
		return spec.Config{}, err
	}

	stepsVal := cfgVal.LookupPath(cue.ParsePath(cueSteps))
	if err := stepsVal.Err(); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.LookupStepsPath failed with an unaccounted-for error-type %T: %w",
				err,
				err,
			))
		}

		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.LookupStepsPath",
		}
	}

	iter, err := stepsVal.List()
	if err != nil {
		panic(errs.BUG("steps is not a list after schema unification: %w", err))
	}

	cfg := spec.Config{
		Unit: unitInst,
	}
	var sawAbort bool
	for iter.Next() {
		idx := iter.Selector().Index()
		stepVal := iter.Value()
		stepSpan, fields := extractFieldSpansFromFile(userFile, idx)

		si, decRes := decodeStep(stepVal, idx, reg, em, stepSpan, fields)
		if decRes.abort {
			sawAbort = true
		}

		if !decRes.ok {
			continue
		}

		cfg.Steps = append(cfg.Steps, si)
	}

	if sawAbort {
		return spec.Config{}, AbortError{}
	}

	return cfg, nil
}

func decodeUnit(
	cfgVal cue.Value,
	cfgPath string,
	userFile *ast.File,
	em diagnostic.Emitter,
) (spec.UnitInstance, error) {
	unitVal := cfgVal.LookupPath(cue.ParsePath(cueUnit))
	if err := unitVal.Err(); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			// unexpected validation failure → engine error
			panic(errs.BUG(
				"load.LookupUnitPath failed with an unaccounted-for error-type %T: %w",
				err,
				err,
			))
		}

		// if no unit is defined, we just default to "anonymous unit"
		if strings.Contains(ce.Error(), "field not found") {
			// return spec.UnitInstance{
			// 	ID:   "anonymous",
			// 	Desc: "automatically generated anonymous unit",
			// }, nil
			return spec.UnitInstance{}, nil
		}

		return spec.UnitInstance{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.LookupUnitPath",
		}
	}

	// Validate
	// ------------------------------------------------------------
	if err := unitVal.Validate(cue.Concrete(true), cue.All()); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.ValidateUnit failed with an unaccounted-for error-type %T: %w",
				err,
				err,
			))
		}

		if isUnitRequiredFieldError(ce) {
			span := extractSpanFromFile(userFile, cueUnit)

			missing := findMissingRequiredFields(ce, unitVal, span)
			if len(missing) > 0 {
				var abort bool
				for _, m := range missing {
					impact, _ := emitEngineDiagnostic(em, cfgPath, MissingFieldDiagnostic{Missing: m})
					if impact.Is(diagnostic.ImpactAbort) {
						abort = true
					}
				}
				if abort {
					return spec.UnitInstance{}, AbortError{Causes: []error{err}}
				}
			}

			impact, _ := emitEngineDiagnostic(em, cfgPath, CueDiagnostic{
				Err:   ce,
				Phase: "decode",
			})

			if impact.Is(diagnostic.ImpactAbort) {
				return spec.UnitInstance{}, AbortError{Causes: []error{err}}
			}
			return spec.UnitInstance{}, nil
		}

		return spec.UnitInstance{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.ValidateUnit",
		}

	}

	// Decode
	// ------------------------------------------------------------
	var unitInst spec.UnitInstance
	if err := unitVal.Decode(&unitInst); err != nil {
		// If Validate passed, Decode MUST NOT fail.
		// This is a hard invariant violation.
		panic(errs.BUG(
			"unitVal.Decode failed after successful validation: %w",
			err,
		))
	}

	return unitInst, nil
}

type decodeResult struct {
	abort bool
	ok    bool
}

func decodeStep(
	stepVal cue.Value,
	stepIdx int,
	reg *Registry,
	em diagnostic.Emitter,
	stepSpan spec.SourceSpan,
	fields map[string]spec.FieldSpan,
) (spec.StepInstance, decodeResult) {
	kind, desc, err := resolveStepKind(stepVal, stepIdx)
	if err != nil {
		// engine/schema error – cannot continue safely
		panic(errs.BUG(
			"resolveStepKind returned an unexpected error for step %q (%s): %w",
			desc,
			kind,
			err,
		))
	}

	// Resolve step type
	// ------------------------------------------------------------
	st, ok := reg.StepType(kind)
	if !ok {
		impact, _ := emitPlanDiagnostic(em, stepIdx, kind, desc, UnknownStepKind{
			Kind:   kind,
			Source: stepSpan,
		})
		return spec.StepInstance{}, decodeResult{
			abort: impact.Is(diagnostic.ImpactAbort),
			ok:    false,
		}
	}

	// Instantiate config
	// ------------------------------------------------------------
	tCfg := st.NewConfig()
	rv := reflect.ValueOf(tCfg)
	if rv.Kind() != reflect.Pointer {
		panic(errs.BUG(
			"StepType['%s'].NewConfig() must return a pointer (got %T)",
			st.Kind(),
			tCfg,
		))
	}

	// Validation
	// ------------------------------------------------------------
	if err := stepVal.Validate(cue.Concrete(true), cue.All()); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			// unexpected validation failure → engine error
			panic(errs.BUG(
				"stepVal.Validate failed with an unaccounted-for error-type %T for step %q (%s): %w",
				err,
				desc,
				kind,
				err,
			))
		}

		missing := findIncompleteFields(
			ce,
			stepVal,
			stepSpan,
		)

		if len(missing) > 0 {
			var abort bool
			for _, m := range missing {
				impact, _ := emitPlanDiagnostic(em, stepIdx, kind, desc, MissingFieldDiagnostic{Missing: m})
				if impact.Is(diagnostic.ImpactAbort) {
					abort = true
				}
			}
			return spec.StepInstance{}, decodeResult{
				abort: abort,
				ok:    false,
			}
		}

		// generic cue validation error, still user-facing
		impact, _ := emitPlanDiagnostic(em, stepIdx, kind, desc, CueDiagnostic{
			Err:   ce,
			Phase: "decode",
		})

		return spec.StepInstance{}, decodeResult{
			abort: impact.Is(diagnostic.ImpactAbort),
			ok:    false,
		}
	}

	// Decode
	// ------------------------------------------------------------
	if err := stepVal.Decode(tCfg); err != nil {
		// If Validate passed, Decode MUST NOT fail.
		// This is a hard invariant violation.
		panic(errs.BUG(
			"stepVal.Decode failed after successful validation for step %q (%s): %w",
			desc,
			kind,
			err,
		))
	}

	// Success
	// ------------------------------------------------------------
	si := spec.StepInstance{
		Desc:   desc,
		Type:   st,
		Config: tCfg,
		Source: stepSpan,
		Fields: fields,
	}

	return si, decodeResult{
		abort: false,
		ok:    true,
	}
}

func resolveStepKind(stepVal cue.Value, idx int) (string, string, error) {
	kindVal := stepVal.LookupPath(cue.ParsePath("kind"))
	if err := kindVal.Err(); err != nil {
		// not found
		return "", "", nil
	}

	kind, err := kindVal.String()
	if err != nil {
		return "", "", err
	}

	if kind == "" {
		return "", "", fmt.Errorf("step at index %d has no kind field", idx)
	}

	// desc is optional - fall back to kind[idx] if not set
	desc, err := stepVal.LookupPath(cue.ParsePath("desc")).String()
	if err != nil {
		desc = fmt.Sprintf("%s[%d]", kind, idx)
	}

	return kind, desc, nil
}

func isErrorWithPath(ce cueerr.Error, searchPath string) bool {
	for _, e := range cueerr.Errors(ce) {
		path := cueerr.Path(e)
		if len(path) == 1 && path[0] == searchPath {
			return true
		}
	}
	return false
}

func isUnitRequiredFieldError(ce cueerr.Error) bool {
	for _, e := range cueerr.Errors(ce) {
		path := cueerr.Path(e)
		if len(path) == 2 && path[0] == cueUnit && path[1] == cueUnitID {
			return true
		}
	}
	return false
}

func isTypeMismatchError(ce cueerr.Error) (string, bool) {
	for _, e := range cueerr.Errors(ce) {
		if strings.Contains(e.Error(), "mismatched types") {
			return strings.Join(cueerr.Path(e), "."), true
		}
	}
	return "", false
}

func describeCueSchemaShape(base cue.Value, path string) string {
	unified := base.FillPath(cue.ParsePath(path), "_")
	v := unified.LookupPath(cue.ParsePath(path))
	return v.Kind().String()
}

func describeCueValueShape(base cue.Value, path string) string {
	v := base.LookupPath(cue.ParsePath(path))
	return v.Kind().String()
}

func getErrorPathSpan(err cueerr.Error, userInst cue.Value) spec.SourceSpan {
	path := cue.ParsePath(strings.Join(cueerr.Path(err), "."))
	v := userInst.LookupPath(path)
	return spanFromPos(v.Pos())
}

func getStepsUnifyErrorSpan(err cueerr.Error, userInst cue.Value) spec.SourceSpan {
	path := cueerr.Path(err)

	// Case 1: error is inside steps[<idx>]
	if len(path) >= 2 && path[0] == cueSteps {
		if idx, err := strconv.Atoi(path[1]); err == nil {
			v := userInst.LookupPath(
				cue.MakePath(cue.Str(cueSteps), cue.Index(idx)),
			)
			return spanFromPos(v.Pos())
		}
	}

	// Case 2: error is about top-level structure (e.g. steps itself)
	if len(path) > 0 {
		v := userInst.LookupPath(cue.MakePath(cue.Str(path[0])))
		return spanFromPos(v.Pos())
	}

	// Case 3: fallback → start-of-file
	return spanFromPos(userInst.Pos())
}

func findMissingRequiredFields(
	ce cueerr.Error,
	stepVal cue.Value,
	stepSource spec.SourceSpan,
) []CueMissingField {
	var res []CueMissingField

	for _, e := range cueerr.Errors(ce) {
		if !strings.Contains(e.Error(), "required but not present") {
			continue
		}

		path := cueerr.Path(e)
		if len(path) == 0 {
			continue
		}

		field := path[len(path)-1]

		v := stepVal.LookupPath(cue.ParsePath(field))
		if !v.Exists() && !v.IsConcrete() {
			res = append(res, CueMissingField{
				Field:  field,
				Source: stepSource,
			})
		}
	}

	return res
}

func findIncompleteFields(
	ce cueerr.Error,
	stepVal cue.Value,
	source spec.SourceSpan,
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

		v := stepVal.LookupPath(cue.ParsePath(field))
		if v.Exists() && !v.IsConcrete() {
			res = append(res, CueMissingField{
				Field:  field,
				Source: source,
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
		Filename:  normalizeVirtualPath(p.Filename),
		StartLine: p.Line,
		StartCol:  p.Column,
		EndCol:    p.Column,
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
		Filename:  normalizeVirtualPath(sp.Filename),
		StartLine: sp.Line,
		EndLine:   ep.Line,
		StartCol:  sp.Column,
		EndCol:    ep.Column,
	}
}

func extractSpanFromFile(f *ast.File, declName string) spec.SourceSpan {
	for _, d := range f.Decls {
		fd, ok := d.(*ast.Field)
		if !ok {
			continue
		}
		id, ok := fd.Label.(*ast.Ident)
		if !ok || id.Name != declName {
			continue
		}

		return spanFromNode(fd)
	}

	return spec.SourceSpan{}
}

func extractFieldSpansFromFile(
	f *ast.File,
	stepIndex int,
) (spec.SourceSpan, map[string]spec.FieldSpan) {
	// 1. locate `steps: [...]`
	var steps *ast.ListLit

	for _, d := range f.Decls {
		fd, ok := d.(*ast.Field)
		if !ok {
			continue
		}
		id, ok := fd.Label.(*ast.Ident)
		if !ok || id.Name != cueSteps {
			continue
		}
		list, ok := fd.Value.(*ast.ListLit)
		if !ok {
			return spec.SourceSpan{}, map[string]spec.FieldSpan{}
		}
		steps = list
		break
	}

	if steps == nil || stepIndex >= len(steps.Elts) {
		return spec.SourceSpan{}, map[string]spec.FieldSpan{}
	}

	// 2. pick the steps expr
	stepExpr := steps.Elts[stepIndex]

	// step may be:
	//   { ... }
	//   builtin.copy & { ... }
	var st *ast.StructLit

	switch e := stepExpr.(type) {
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

	stepSpan := spanFromNode(stepExpr)
	return stepSpan, fields
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
