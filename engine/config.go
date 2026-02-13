// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"reflect"
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
	cueTargets = "targets"
	cueDeploy  = "deploy"
	cueSteps   = "steps"
	cueAttrEnv = "env"
)

// LoadConfig decodes and validates user configuration.
// It returns ONLY user-facing configuration errors.
// All other failures are engine or environment bugs and will panic.
func LoadConfig(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	src source.Source,
) (cfg spec.Config, err error) {
	cfgPath, absErr := filepath.Abs(cfgPath)
	if absErr != nil {
		panic(errs.BUG("filepath.Abs() failed: %w", absErr))
	}

	cfg, err = loadConfigWithSource(ctx, em, cfgPath, store, src)
	if err != nil {
		impact, _ := emitEngineDiagnostic(em, cfgPath, err)
		if impact.ShouldAbort() {
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
			panicErr := CuePanicError{Recovered: r}
			_, _ = emitEngineDiagnostic(em, cfgPath, panicErr)
			err = AbortError{Causes: []error{panicErr}}
		}
	}()

	cfg, err = loadConfigPipeline(ctx, em, cfgPath, nil, store, src)
	return
}

func loadConfigPipeline(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	additionalPaths []string,
	store *spec.SourceStore,
	src source.Source,
) (spec.Config, error) {
	reg := NewRegistry()
	cueCtx := cuecontext.New()

	embFS, err := fs.Sub(doit.EmbeddedSchemaModule, "cue")
	if err != nil {
		panic(errs.BUG("embedded schema FS corrupted: %w", err))
	}

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

	// Load user config
	// -----------------------------------------------------------------------------
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

	// Unify additional files (inventory, vars) if any
	// -----------------------------------------------------------------------------
	for _, addPath := range additionalPaths {
		if _, err := src.ReadFile(ctx, addPath); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return spec.Config{}, InventoryNotFoundError{Path: addPath}
		}

		addInstances := load.Instances([]string{addPath}, loaderCfg)
		if len(addInstances) == 0 {
			return spec.Config{}, InventoryNotFoundError{Path: addPath}
		}
		addInstance := addInstances[0]
		if err := addInstance.Err; err != nil {
			var ce cueerr.Error
			if !errors.As(err, &ce) {
				panic(errs.BUG(
					"load.Instances returned an unexpected error for additional path %q: %w",
					addPath,
					err,
				))
			}
			return spec.Config{}, CueDiagnostic{
				Err:   ce,
				Phase: "load.additionalInstances",
			}
		}

		addInst := cueCtx.BuildInstance(addInstance)
		if err := addInst.Err(); err != nil {
			var ce cueerr.Error
			if !errors.As(err, &ce) {
				panic(errs.BUG(
					"load.BuildAdditionalInstance returned an unexpected error for %q: %w",
					addPath,
					err,
				))
			}
			return spec.Config{}, CueDiagnostic{
				Err:   ce,
				Phase: "load.BuildAdditionalInstance",
			}
		}

		userInst = userInst.Unify(addInst)
		if err := userInst.Err(); err != nil {
			var ce cueerr.Error
			if !errors.As(err, &ce) {
				panic(errs.BUG(
					"unify failed with an unaccounted-for error-type %T: %w",
					err,
					err,
				))
			}
			return spec.Config{}, CueDiagnostic{
				Err:   ce,
				Phase: "load.UnifyAdditional",
			}
		}
	}

	// Load core schema and apply
	// -----------------------------------------------------------------------------
	coreInstances := load.Instances([]string{"godoit.dev/doit/core"}, loaderCfg)
	if len(coreInstances) == 0 {
		panic(errs.BUG("load.Instances returned zero core-instances"))
	}
	if err := coreInstances[0].Err; err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.CoreInstances returned an unexpected error: %w",
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
				"load.BuildCoreInstance returned an unexpected error: %w",
				err,
			))
		}
		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.BuildCoreInstance",
		}
	}

	cfgVal := coreInst.Value().Unify(userInst)
	if err := cfgVal.Err(); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.Unify failed with an unaccounted-for error-type %T: %w",
				err,
				err,
			))
		}

		if path, ok := isTypeMismatchError(ce); ok {
			return spec.Config{}, TypeMismatchError{
				Source: getErrorPathSpan(ce, userInst),
				Path:   path,
				Have:   describeCueValueShape(userInst, path),
				Want:   describeCueSchemaShape(coreInst, path),
			}
		}

		if path, ok := isFieldNotAllowedError(ce); ok {
			return spec.Config{}, FieldNotAllowedError{
				Source: getErrorPathSpan(ce, userInst),
				Path:   path,
			}
		}

		panic(errs.BUG(
			"load.Unify produced an unclassified CUE error: %w",
			ce,
		))
	}

	// Decode config
	// -----------------------------------------------------------------------------
	userFile := userInstance.Files[0]

	cfg := spec.Config{
		Path:    cfgPath,
		Targets: make(map[string]spec.TargetInstance),
		Deploy:  make(map[string]spec.DeployBlock),
	}

	targetsVal := cfgVal.LookupPath(cue.ParsePath(cueTargets))
	if err := targetsVal.Err(); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.LookupTargetsPath failed with an unaccounted-for error-type %T: %w",
				err,
				err,
			))
		}
		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.LookupTargetsPath",
		}
	}

	targetsIter, err := targetsVal.Fields()
	if err != nil {
		panic(errs.BUG("targets is not a struct after schema unification: %w", err))
	}

	for targetsIter.Next() {
		targetName := targetsIter.Selector().String()
		targetVal := targetsIter.Value()

		tgtInst, err := decodeTargetValue(targetVal, userFile, reg, src)
		if err != nil {
			return spec.Config{}, err
		}
		cfg.Targets[targetName] = tgtInst
	}

	deployVal := cfgVal.LookupPath(cue.ParsePath(cueDeploy))
	if err := deployVal.Err(); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"load.LookupDeployPath failed with an unaccounted-for error-type %T: %w",
				err,
				err,
			))
		}
		return spec.Config{}, CueDiagnostic{
			Err:   ce,
			Phase: "load.LookupDeployPath",
		}
	}

	deployIter, err := deployVal.Fields()
	if err != nil {
		panic(errs.BUG("deploy is not a struct after schema unification: %w", err))
	}

	var sawAbort bool
	for deployIter.Next() {
		blockName := deployIter.Selector().String()
		blockVal := deployIter.Value()

		block, blockAbort, err := decodeDeployBlock(blockVal, blockName, reg, em, userFile)
		if err != nil {
			return spec.Config{}, err
		}
		if blockAbort {
			sawAbort = true
		}
		cfg.Deploy[blockName] = block
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

func decodeStep(
	stepVal cue.Value,
	stepIdx int,
	reg *Registry,
	em diagnostic.Emitter,
	stepSpan spec.SourceSpan,
	fields map[string]spec.FieldSpan,
	typeRef string,
) (spec.StepInstance, decodeResult) {
	kind, desc, err := resolveStepKind(stepVal, stepIdx)
	if err != nil {
		// The step value is unresolvable (e.g. builtin.directory doesn't
		// exist). Report it as an unknown step type using the source
		// expression rather than leaking internal CUE field names.
		ref := typeRef
		if ref == "" {
			ref = kind
		}
		impact, _ := emitPlanDiagnostic(em, stepIdx, kind, desc, UnknownStepKindError{
			Kind:   ref,
			Source: stepSpan,
		})
		return spec.StepInstance{}, decodeResult{
			abort: impact.ShouldAbort(),
			ok:    false,
		}
	}

	// Resolve step type
	// -----------------------------------------------------------------------------
	st, ok := reg.StepType(kind)
	if !ok {
		impact, _ := emitPlanDiagnostic(em, stepIdx, kind, desc, UnknownStepKindError{
			Kind:   kind,
			Source: stepSpan,
		})
		return spec.StepInstance{}, decodeResult{
			abort: impact.ShouldAbort(),
			ok:    false,
		}
	}

	// Instantiate config
	// -----------------------------------------------------------------------------
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
	// -----------------------------------------------------------------------------
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
				if impact.ShouldAbort() {
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
			abort: impact.ShouldAbort(),
			ok:    false,
		}
	}

	// Decode
	// -----------------------------------------------------------------------------
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
	// -----------------------------------------------------------------------------
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

func decodeTargetValue(
	targetVal cue.Value,
	userFile *ast.File,
	reg *Registry,
	src source.Source,
) (spec.TargetInstance, error) {
	kindVal := targetVal.LookupPath(cue.ParsePath("kind"))
	kind, err := kindVal.String()
	if err != nil || kind == "" {
		panic(errs.BUG(
			"target missing kind after CUE schema validation: %w",
			err,
		))
	}

	tt, ok := reg.TargetType(kind)
	if !ok {
		panic(errs.BUG(
			"unknown target kind %q passed CUE schema validation",
			kind,
		))
	}

	tCfg := tt.NewConfig()
	rv := reflect.ValueOf(tCfg)
	if rv.Kind() != reflect.Pointer {
		panic(errs.BUG(
			"TargetType['%s'].NewConfig() must return a pointer (got %T)",
			tt.Kind(),
			tCfg,
		))
	}

	envMap := extractEnvMap(targetVal)
	targetVal, err = fillNonConcreteFromEnv(targetVal, envMap, src)
	if err != nil {
		return spec.TargetInstance{}, err
	}

	if err := targetVal.Validate(cue.Concrete(true), cue.All()); err != nil {
		var ce cueerr.Error
		if !errors.As(err, &ce) {
			panic(errs.BUG(
				"targetVal.Validate failed with an unaccounted-for error-type %T for target %q: %w",
				err,
				kind,
				err,
			))
		}

		targetSpan := extractSpanFromFile(userFile, cueTargets)
		missing := findIncompleteFields(ce, targetVal, targetSpan)

		if len(missing) > 0 {
			var diags diagnostic.Diagnostics
			for _, m := range missing {
				d := MissingFieldDiagnostic{Missing: m}
				diags = append(diags, d)
			}
			return spec.TargetInstance{}, diags
		}

		return spec.TargetInstance{}, CueDiagnostic{
			Err:   ce,
			Phase: "decode",
		}
	}

	if err := targetVal.Decode(tCfg); err != nil {
		panic(errs.BUG(
			"targetVal.Decode failed after successful validation for target %q: %w",
			kind,
			err,
		))
	}

	if err := applyEnvOverridesToStruct(tCfg, envMap, src); err != nil {
		return spec.TargetInstance{}, err
	}

	return spec.TargetInstance{
		Type:   tt,
		Config: tCfg,
	}, nil
}

func decodeDeployBlock(
	blockVal cue.Value,
	blockName string,
	reg *Registry,
	em diagnostic.Emitter,
	userFile *ast.File,
) (spec.DeployBlock, bool, error) {
	block := spec.DeployBlock{
		Name: blockName,
	}

	targetsVal := blockVal.LookupPath(cue.ParsePath("targets"))
	if err := targetsVal.Err(); err != nil {
		var ce cueerr.Error
		if errors.As(err, &ce) {
			return block, false, CueDiagnostic{
				Err:   ce,
				Phase: "decode.deploy.targets",
			}
		}
		panic(errs.BUG("deploy block targets lookup failed: %w", err))
	}

	targetsIter, err := targetsVal.List()
	if err != nil {
		panic(errs.BUG("deploy block targets is not a list: %w", err))
	}

	for targetsIter.Next() {
		targetName, err := targetsIter.Value().String()
		if err != nil {
			panic(errs.BUG("deploy block target name is not a string: %w", err))
		}
		block.Targets = append(block.Targets, targetName)
	}

	stepsVal := blockVal.LookupPath(cue.ParsePath(cueSteps))
	if err := stepsVal.Err(); err != nil {
		var ce cueerr.Error
		if errors.As(err, &ce) {
			return block, false, CueDiagnostic{
				Err:   ce,
				Phase: "decode.deploy.steps",
			}
		}
		panic(errs.BUG("deploy block steps lookup failed: %w", err))
	}

	stepsIter, err := stepsVal.List()
	if err != nil {
		panic(errs.BUG("deploy block steps is not a list: %w", err))
	}

	stepNodes := findStepNodes(userFile, blockName)

	var sawAbort bool
	for stepsIter.Next() {
		idx := stepsIter.Selector().Index()
		stepVal := stepsIter.Value()

		var stepSpan spec.SourceSpan
		var fields map[string]spec.FieldSpan
		var typeRef string
		if idx < len(stepNodes) {
			stepSpan = spanFromNode(stepNodes[idx])
			fields = extractFieldSpansFromAST(stepNodes[idx])
			typeRef = stepTypeRef(stepNodes[idx])
		}
		if len(fields) == 0 {
			fields = extractFieldSpans(stepVal)
		}

		si, decRes := decodeStep(stepVal, idx, reg, em, stepSpan, fields, typeRef)
		if decRes.abort {
			sawAbort = true
		}
		if !decRes.ok {
			continue
		}
		block.Steps = append(block.Steps, si)
	}

	return block, sawAbort, nil
}

func resolveStepKind(stepVal cue.Value, idx int) (string, string, error) {
	// Check if the step value itself is bottom (e.g. bad reference like
	// builtin.directory when the field doesn't exist). Surface that error
	// rather than the misleading "kind not found" from looking up a field
	// on a bottom value.
	if err := stepVal.Err(); err != nil {
		return "", "", err
	}

	kindVal := stepVal.LookupPath(cue.ParsePath("kind"))
	if err := kindVal.Err(); err != nil {
		return "", "", err
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

func isTypeMismatchError(ce cueerr.Error) (string, bool) {
	for _, e := range cueerr.Errors(ce) {
		if strings.Contains(e.Error(), "mismatched types") {
			return strings.Join(cueerr.Path(e), "."), true
		}
	}
	return "", false
}

func isFieldNotAllowedError(ce cueerr.Error) (string, bool) {
	for _, e := range cueerr.Errors(ce) {
		if strings.Contains(e.Error(), "field not allowed") {
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

func findIncompleteFields(
	ce cueerr.Error,
	val cue.Value,
	source spec.SourceSpan,
) []CueMissingField {
	envMap := extractEnvMap(val)

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

		v := val.LookupPath(cue.ParsePath(field))
		if v.Exists() && !v.IsConcrete() {
			var envVar *string
			if e, ok := envMap[field]; ok {
				envVar = &e
			}
			res = append(res, CueMissingField{
				Field:  field,
				Env:    envVar,
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

func extractFieldSpans(v cue.Value) map[string]spec.FieldSpan {
	fields := make(map[string]spec.FieldSpan)
	iter, err := v.Fields()
	if err != nil {
		return fields
	}
	for iter.Next() {
		name := iter.Selector().String()
		fields[name] = spec.FieldSpan{
			Value: spanFromPos(iter.Value().Pos()),
		}
	}
	return fields
}

// findStepNodes locates the AST list elements for deploy.<blockName>.steps
// in the user's source file. Returns nil if the path cannot be resolved.
func findStepNodes(f *ast.File, blockName string) []ast.Expr {
	deployLit := findFieldStruct(f.Decls, "deploy")
	if deployLit == nil {
		return nil
	}
	blockLit := findFieldStruct(deployLit.Elts, blockName)
	if blockLit == nil {
		return nil
	}
	stepsField := findFieldValue(blockLit.Elts, cueSteps)
	if stepsField == nil {
		return nil
	}
	list, ok := stepsField.(*ast.ListLit)
	if !ok {
		return nil
	}
	return list.Elts
}

// findFieldStruct looks up a field by name and returns its value as a StructLit.
func findFieldStruct(decls []ast.Decl, name string) *ast.StructLit {
	v := findFieldValue(decls, name)
	if v == nil {
		return nil
	}
	sl, ok := v.(*ast.StructLit)
	if !ok {
		return nil
	}
	return sl
}

// findFieldValue looks up a field by name and returns its value expression.
func findFieldValue(decls []ast.Decl, name string) ast.Expr {
	for _, d := range decls {
		fd, ok := d.(*ast.Field)
		if !ok {
			continue
		}
		id, ok := fd.Label.(*ast.Ident)
		if !ok {
			continue
		}
		if id.Name == name {
			return fd.Value
		}
	}
	return nil
}

// extractFieldSpansFromAST extracts field spans from a step's AST node.
// It finds the struct literal (possibly nested inside a unification expr)
// and maps each field label to a FieldSpan with the value's source position.
func extractFieldSpansFromAST(expr ast.Expr) map[string]spec.FieldSpan {
	sl := findStructLit(expr)
	if sl == nil {
		return nil
	}
	fields := make(map[string]spec.FieldSpan, len(sl.Elts))
	for _, d := range sl.Elts {
		fd, ok := d.(*ast.Field)
		if !ok {
			continue
		}
		id, ok := fd.Label.(*ast.Ident)
		if !ok {
			continue
		}
		fields[id.Name] = spec.FieldSpan{
			Value: spanFromNode(fd.Value),
		}
	}
	return fields
}

// findStructLit finds the struct literal in an expression, descending into
// the right-hand side of binary expressions (e.g. builtin.copy & { ... }).
func findStructLit(expr ast.Expr) *ast.StructLit {
	switch v := expr.(type) {
	case *ast.StructLit:
		return v
	case *ast.BinaryExpr:
		if sl, ok := v.Y.(*ast.StructLit); ok {
			return sl
		}
		if sl, ok := v.X.(*ast.StructLit); ok {
			return sl
		}
	}
	return nil
}

// stepTypeRef extracts the type reference from a step AST expression.
// For "builtin.copy & { ... }" it returns "builtin.copy".
func stepTypeRef(expr ast.Expr) string {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok {
		return ""
	}
	return exprString(bin.X)
}

func exprString(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		sel, ok := v.Sel.(*ast.Ident)
		if !ok {
			return ""
		}
		parent := exprString(v.X)
		if parent == "" {
			return sel.Name
		}
		return parent + "." + sel.Name
	default:
		return ""
	}
}

func extractSpanFromFile(f *ast.File, declName string) spec.SourceSpan {
	var field *ast.Field

	for _, d := range f.Decls {
		fd, ok := d.(*ast.Field)
		if !ok {
			continue
		}
		id, ok := fd.Label.(*ast.Ident)
		if !ok || id.Name != declName {
			continue
		}

		field = fd
		break
	}

	if field == nil {
		return spec.SourceSpan{}
	}

	return spanFromNode(field)
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

// LoadConfigWithOptions loads configuration with optional inventory/vars files.
func LoadConfigWithOptions(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	store *spec.SourceStore,
	src source.Source,
	opts spec.ResolveOptions,
) (spec.Config, error) {
	cfgPath, absErr := filepath.Abs(cfgPath)
	if absErr != nil {
		panic(errs.BUG("filepath.Abs() failed: %w", absErr))
	}

	additionalPaths := collectAdditionalPaths(cfgPath, opts)

	if len(additionalPaths) == 0 {
		return LoadConfig(ctx, em, cfgPath, store, src)
	}

	cfg, err := loadConfigWithAdditionalFiles(ctx, em, cfgPath, additionalPaths, store, src)
	if err != nil {
		impact, _ := emitEngineDiagnostic(em, cfgPath, err)
		if impact.ShouldAbort() {
			return spec.Config{}, AbortError{Causes: []error{err}}
		}
		return spec.Config{}, panicIfNotAbortError(err)
	}

	return cfg, nil
}

// collectAdditionalPaths determines which additional CUE files to load based on options.
func collectAdditionalPaths(cfgPath string, opts spec.ResolveOptions) []string {
	cfgDir := filepath.Dir(cfgPath)
	var paths []string

	// Explicit inventory path takes precedence
	if opts.InventoryPath != "" {
		invPath := opts.InventoryPath
		if !filepath.IsAbs(invPath) {
			invPath = filepath.Join(cfgDir, invPath)
		}
		paths = append(paths, invPath)
	}

	// --env loads inventory/<name>.cue and vars/<name>.cue
	if opts.EnvName != "" {
		invPath := filepath.Join(cfgDir, "inventory", opts.EnvName+".cue")
		paths = append(paths, invPath)

		varsPath := filepath.Join(cfgDir, "vars", opts.EnvName+".cue")
		// vars file is optional - will be checked during load
		paths = append(paths, varsPath)
	}

	return paths
}

func loadConfigWithAdditionalFiles(
	ctx context.Context,
	em diagnostic.Emitter,
	cfgPath string,
	additionalPaths []string,
	store *spec.SourceStore,
	src source.Source,
) (cfg spec.Config, err error) {
	// Guard against panics in the CUE library
	defer func() {
		if r := recover(); r != nil {
			panicErr := CuePanicError{Recovered: r}
			_, _ = emitEngineDiagnostic(em, cfgPath, panicErr)
			err = AbortError{Causes: []error{panicErr}}
		}
	}()

	return loadConfigPipeline(ctx, em, cfgPath, additionalPaths, store, src)
}

// ResolveMultiple produces ResolvedConfigs for all matching (deploy, target)
// combinations based on the provided options.
func ResolveMultiple(cfg spec.Config, opts spec.ResolveOptions) ([]spec.ResolvedConfig, error) {
	var deployNames []string
	if len(opts.DeployNames) > 0 {
		for _, name := range opts.DeployNames {
			if _, ok := cfg.Deploy[name]; !ok {
				return nil, UnknownDeployBlockError{Name: name}
			}
		}
		deployNames = opts.DeployNames
	} else {
		for name := range cfg.Deploy {
			deployNames = append(deployNames, name)
		}
	}

	if len(deployNames) == 0 {
		return nil, NoDeployBlocksError{}
	}

	var results []spec.ResolvedConfig

	for _, deployName := range deployNames {
		block := cfg.Deploy[deployName]

		var targetNames []string
		if len(opts.TargetNames) > 0 {
			blockTargets := make(map[string]bool)
			for _, t := range block.Targets {
				blockTargets[t] = true
			}
			for _, t := range opts.TargetNames {
				if blockTargets[t] {
					targetNames = append(targetNames, t)
				}
			}
			if len(targetNames) == 0 {
				continue
			}
		} else {
			targetNames = block.Targets
		}

		if len(targetNames) == 0 {
			return nil, NoTargetsInDeployError{Deploy: deployName}
		}

		for _, targetName := range targetNames {
			tgt, ok := cfg.Targets[targetName]
			if !ok {
				return nil, UnknownTargetError{
					Name:   targetName,
					Deploy: deployName,
				}
			}

			results = append(results, spec.ResolvedConfig{
				Path:       cfg.Path,
				DeployName: deployName,
				TargetName: targetName,
				Target:     tgt,
				Steps:      block.Steps,
				Sources:    cfg.Sources,
			})
		}
	}

	if len(results) == 0 {
		return nil, NoDeployBlocksError{}
	}

	return results, nil
}

// Resolve produces a ResolvedConfig from a Config by selecting a specific
// deploy block and target. If deployName or targetName are empty, the first
// available is selected.
func Resolve(cfg spec.Config, deployName, targetName string) (spec.ResolvedConfig, error) {
	var block spec.DeployBlock
	if deployName != "" {
		var ok bool
		block, ok = cfg.Deploy[deployName]
		if !ok {
			return spec.ResolvedConfig{}, UnknownDeployBlockError{Name: deployName}
		}
	} else {
		// Pick first deploy block (map iteration order is random, but for
		// single-block configs this is fine)
		for name, b := range cfg.Deploy {
			block = b
			deployName = name
			break
		}
		if deployName == "" {
			return spec.ResolvedConfig{}, NoDeployBlocksError{}
		}
	}

	if targetName == "" {
		if len(block.Targets) == 0 {
			return spec.ResolvedConfig{}, NoTargetsInDeployError{Deploy: deployName}
		}
		targetName = block.Targets[0]
	}

	// Verify target exists in config
	tgt, ok := cfg.Targets[targetName]
	if !ok {
		return spec.ResolvedConfig{}, UnknownTargetError{
			Name:   targetName,
			Deploy: deployName,
		}
	}

	// Verify target is in the deploy block's target list
	var found bool
	for _, t := range block.Targets {
		if t == targetName {
			found = true
			break
		}
	}
	if !found {
		return spec.ResolvedConfig{}, TargetNotInDeployError{
			Target: targetName,
			Deploy: deployName,
		}
	}

	return spec.ResolvedConfig{
		Path:       cfg.Path,
		DeployName: deployName,
		TargetName: targetName,
		Target:     tgt,
		Steps:      block.Steps,
		Sources:    cfg.Sources,
	}, nil
}
