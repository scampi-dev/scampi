// SPDX-License-Identifier: GPL-3.0-only

// Package linker bridges the scampi frontend and the engine.
// It takes the evaluator's generic Result (StructVals, BlockResultVals)
// and resolves them against the engine registry to produce spec.DeclaredConfig.
package linker

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"scampi.dev/scampi/internal/lang/eval"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/spec"
)

// Registry provides step and target type lookups. Implemented by
// engine.Registry — defined as an interface here to avoid a cycle.
type Registry interface {
	StepKind(kind string) (spec.StepKind, bool)
	TargetKind(kind string) (spec.TargetKind, bool)
	ConverterFor(reflect.Type) (spec.TypeConverter, bool)
}

// LinkOption configures the linker.
type LinkOption func(*linkConfig)

type linkConfig struct {
	ctx          context.Context
	cfgPath      string
	src          source.Source
	source       []byte
	converterFor func(reflect.Type) (spec.TypeConverter, bool)
}

// WithSourceResolver enables source resolution (inline caching,
// local path resolution) during linking.
func WithSourceResolver(ctx context.Context, cfgPath string, src source.Source) LinkOption {
	return func(lc *linkConfig) {
		lc.ctx = ctx
		lc.cfgPath = cfgPath
		lc.src = src
	}
}

// WithSource provides the raw source bytes of the entry-point config
// file. Required for translating eval-side token offsets into
// line/col coordinates on DeclaredStep.Source and DeclaredStep.Fields.
// Without it, linked instances carry zero-valued spans.
func WithSource(data []byte) LinkOption {
	return func(lc *linkConfig) {
		lc.source = data
	}
}

// Link converts a lang eval result into a spec.DeclaredConfig by walking all
// values, interpreting them based on RetType/TypeName, and resolving
// step and target names against the engine registry.
func Link(result *eval.Result, reg Registry, path string, opts ...LinkOption) (spec.DeclaredConfig, error) {
	lc := linkConfig{cfgPath: path}
	for _, o := range opts {
		o(&lc)
	}
	lc.converterFor = reg.ConverterFor
	cfg := spec.DeclaredConfig{
		Path:    path,
		Targets: make(map[string]spec.DeclaredTarget),
	}

	var linkErr error
	eval.WalkResult(result, func(v eval.Value) bool {
		if linkErr != nil {
			return false
		}
		switch sv := v.(type) {
		case *eval.StructVal:
			linkErr = linkStructVal(sv, result, reg, &cfg, &lc)
			return false // don't descend into fields
		case *eval.BlockResultVal:
			linkErr = linkBlockResult(sv, reg, &cfg, &lc)
			return false
		}
		return true
	})

	return cfg, linkErr
}

func linkStructVal(sv *eval.StructVal, r *eval.Result, reg Registry, cfg *spec.DeclaredConfig, lc *linkConfig) error {
	switch sv.RetType {
	case "Target":
		ti, err := linkTarget(sv, reg, lc)
		if err != nil {
			return err
		}
		targetName := bindingName(r, sv)
		if n, ok := sv.Fields["name"].(*eval.StringVal); ok {
			targetName = n.V
		}
		cfg.Targets[targetName] = ti
	}
	return nil
}

// bindingName returns the let-binding name for a value, or "" if it's
// a bare expression.
func bindingName(r *eval.Result, v eval.Value) string {
	for name, bv := range r.Bindings {
		if bv == v {
			return name
		}
	}
	return ""
}

func linkBlockResult(bv *eval.BlockResultVal, reg Registry, cfg *spec.DeclaredConfig, lc *linkConfig) error {
	switch bv.TypeName {
	case "Deploy":
		db, err := linkDeploy(bv, reg, lc)
		if err != nil {
			return err
		}
		cfg.Deploy = append(cfg.Deploy, db)
	}
	return nil
}

// structValSpans converts the eval-side token spans on a StructVal
// into a (Source, Fields) pair the engine can use for diagnostics. If
// the link config has no source bytes, returns zero values — callers
// that only care about Type/Config see the same DeclaredStep shape as
// before this plumbing existed.
func structValSpans(sv *eval.StructVal, lc *linkConfig) (spec.SourceSpan, map[string]spec.FieldSpan) {
	if len(lc.source) == 0 {
		return spec.SourceSpan{}, nil
	}
	source := spec.SourceSpan{Filename: lc.cfgPath}
	if sv.SrcSpan.End > 0 {
		sLine, sCol := offsetToLineCol(lc.source, int(sv.SrcSpan.Start))
		eLine, eCol := offsetToLineCol(lc.source, int(sv.SrcSpan.End))
		source.StartLine = sLine
		source.StartCol = sCol
		source.EndLine = eLine
		source.EndCol = eCol
	}
	fields := make(map[string]spec.FieldSpan, len(sv.FieldSpans))
	for name, span := range sv.FieldSpans {
		if span.End == 0 {
			continue
		}
		sLine, sCol := offsetToLineCol(lc.source, int(span.Start))
		eLine, eCol := offsetToLineCol(lc.source, int(span.End))
		fields[name] = spec.FieldSpan{
			Value: spec.SourceSpan{
				Filename:  lc.cfgPath,
				StartLine: sLine,
				StartCol:  sCol,
				EndLine:   eLine,
				EndCol:    eCol,
			},
		}
	}
	return source, fields
}

func linkTarget(sv *eval.StructVal, reg Registry, lc *linkConfig) (spec.DeclaredTarget, error) {
	// Try leaf name (e.g. "ssh"), qualified (e.g. "rest.target"),
	// then module prefix (e.g. "rest" from "rest.target").
	tt, ok := reg.TargetKind(sv.TypeName)
	if !ok {
		tt, ok = reg.TargetKind(sv.QualName)
	}
	if !ok && strings.Contains(sv.QualName, ".") {
		mod := sv.QualName[:strings.IndexByte(sv.QualName, '.')]
		tt, ok = reg.TargetKind(mod)
	}
	if !ok {
		return spec.DeclaredTarget{}, &UnresolvedError{Kind: "target", Name: sv.TypeName}
	}
	cfg := tt.NewConfig()
	if err := mapFields(sv.Fields, cfg, lc); err != nil {
		return spec.DeclaredTarget{}, err
	}
	source, fields := structValSpans(sv, lc)
	return spec.DeclaredTarget{
		Type:   tt,
		Config: cfg,
		Source: source,
		Fields: fields,
	}, nil
}

func linkDeploy(bv *eval.BlockResultVal, reg Registry, lc *linkConfig) (spec.DeclaredDeploy, error) {
	db := spec.DeclaredDeploy{
		Hooks: make(map[string][]spec.DeclaredStep),
	}

	if n, ok := bv.Fields["name"].(*eval.StringVal); ok {
		db.Name = n.V
	}
	if t, ok := bv.Fields["targets"].(*eval.ListVal); ok {
		for _, item := range t.Items {
			if sv, ok := item.(*eval.StructVal); ok {
				if n, ok := sv.Fields["name"].(*eval.StringVal); ok {
					db.Targets = append(db.Targets, n.V)
				}
			}
		}
	}

	// Track hook IDs by StructVal pointer to deduplicate.
	hookIDs := map[*eval.StructVal]string{}
	hookCounter := 0

	// extractHooks recursively processes on_change fields and
	// registers hooks in the deploy block.
	var extractHooks func(sv *eval.StructVal, si *spec.DeclaredStep) error
	extractHooks = func(sv *eval.StructVal, si *spec.DeclaredStep) error {
		oc, ok := sv.Fields["on_change"]
		if !ok {
			return nil
		}
		list, ok := oc.(*eval.ListVal)
		if !ok {
			return nil
		}
		for _, hookVal := range list.Items {
			hookSV, ok := hookVal.(*eval.StructVal)
			if !ok {
				continue
			}
			hid, exists := hookIDs[hookSV]
			if !exists {
				hookStep, err := linkStep(hookSV, reg, lc)
				if err != nil {
					return err
				}
				// Recursively wire this hook's own on_change.
				if err := extractHooks(hookSV, &hookStep); err != nil {
					return err
				}
				hookCounter++
				hid = fmt.Sprintf("hook-%d", hookCounter)
				hookIDs[hookSV] = hid
				db.Hooks[hid] = []spec.DeclaredStep{hookStep}
			}
			si.OnChange = append(si.OnChange, hid)
		}
		return nil
	}

	// Link steps and assign sequential StepIDs. Build a mapping
	// from eval-time StructVal pointer to StepID so we can resolve
	// ref() values after all steps have been linked.
	stepIDs := map[*eval.StructVal]spec.StepID{}
	var nextID spec.StepID = 1

	for _, v := range bv.Body {
		sv, ok := v.(*eval.StructVal)
		if !ok {
			continue
		}
		si, err := linkStep(sv, reg, lc)
		if err != nil {
			return db, err
		}
		si.ID = nextID
		stepIDs[sv] = nextID
		nextID++
		if err := extractHooks(sv, &si); err != nil {
			return db, err
		}
		db.Steps = append(db.Steps, si)
	}

	// Resolve ref() values: walk all step configs and replace
	// RefVals with spec.Refs using the StepID mapping.
	for i := range db.Steps {
		resolveStepRefs(&db.Steps[i], stepIDs)
	}

	return db, nil
}

// resolveStepRefs walks a DeclaredStep's config looking for RefVal
// values (produced by std.ref()) and replaces them with spec.Ref
// using the StructVal → StepID mapping from linking. RefVals can
// appear in map[string]any fields because the linker's setValue
// converts eval.Value → Go native types, but RefVal is special —
// it passes through as-is (narrow interface path).
func resolveStepRefs(si *spec.DeclaredStep, ids map[*eval.StructVal]spec.StepID) {
	if si.Config == nil {
		return
	}
	v := reflect.ValueOf(si.Config)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	for _, fv := range v.Fields() {
		if !fv.CanInterface() {
			continue
		}
		switch val := fv.Interface().(type) {
		case *eval.RefVal:
			if val != nil {
				targetID := ids[val.Step]
				fv.Set(reflect.ValueOf(spec.Ref{
					TargetID: targetID,
					Expr:     val.Expr,
				}))
			}
		case map[string]any:
			resolveMapRefs(val, ids)
		}
	}
}

func resolveMapRefs(m map[string]any, ids map[*eval.StructVal]spec.StepID) {
	for k, v := range m {
		if rv, ok := v.(*eval.RefVal); ok && rv != nil {
			m[k] = spec.Ref{
				TargetID: ids[rv.Step],
				Expr:     rv.Expr,
			}
		}
	}
}

func linkStep(sv *eval.StructVal, reg Registry, lc *linkConfig) (spec.DeclaredStep, error) {
	// Try qualified name first (e.g. "container.instance"), then leaf.
	name := sv.QualName
	st, ok := reg.StepKind(name)
	if !ok {
		name = sv.TypeName
		st, ok = reg.StepKind(name)
	}
	if !ok {
		return spec.DeclaredStep{}, &UnresolvedError{Kind: "step", Name: sv.TypeName}
	}
	cfg := st.NewConfig()
	if err := mapFields(sv.Fields, cfg, lc); err != nil {
		return spec.DeclaredStep{}, err
	}

	var desc string
	if d, ok := sv.Fields["desc"].(*eval.StringVal); ok {
		desc = d.V
	}

	source, fields := structValSpans(sv, lc)
	return spec.DeclaredStep{
		Type:   st,
		Config: cfg,
		Desc:   desc,
		Source: source,
		Fields: fields,
	}, nil
}
