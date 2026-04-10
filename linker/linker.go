// SPDX-License-Identifier: GPL-3.0-only

// Package linker bridges the scampi frontend and the engine.
// It takes the evaluator's generic Result (StructVals, BlockResultVals)
// and resolves them against the engine registry to produce spec.Config.
package linker

import (
	"context"
	"fmt"
	"strings"

	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
)

// Registry provides step and target type lookups. Implemented by
// engine.Registry — defined as an interface here to avoid a cycle.
type Registry interface {
	StepType(kind string) (spec.StepType, bool)
	TargetType(kind string) (spec.TargetType, bool)
}

// LinkOption configures the linker.
type LinkOption func(*linkConfig)

type linkConfig struct {
	ctx     context.Context
	cfgPath string
	src     source.Source
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

// Link converts a lang eval result into a spec.Config by walking all
// values, interpreting them based on RetType/TypeName, and resolving
// step and target names against the engine registry.
func Link(result *eval.Result, reg Registry, path string, opts ...LinkOption) (spec.Config, error) {
	var lc linkConfig
	for _, o := range opts {
		o(&lc)
	}
	cfg := spec.Config{
		Path:    path,
		Targets: make(map[string]spec.TargetInstance),
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

func linkStructVal(sv *eval.StructVal, r *eval.Result, reg Registry, cfg *spec.Config, lc *linkConfig) error {
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

func linkBlockResult(bv *eval.BlockResultVal, reg Registry, cfg *spec.Config, lc *linkConfig) error {
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

func linkTarget(sv *eval.StructVal, reg Registry, lc *linkConfig) (spec.TargetInstance, error) {
	// Try leaf name (e.g. "ssh"), qualified (e.g. "rest.target"),
	// then module prefix (e.g. "rest" from "rest.target").
	tt, ok := reg.TargetType(sv.TypeName)
	if !ok {
		tt, ok = reg.TargetType(sv.QualName)
	}
	if !ok && strings.Contains(sv.QualName, ".") {
		mod := sv.QualName[:strings.IndexByte(sv.QualName, '.')]
		tt, ok = reg.TargetType(mod)
	}
	if !ok {
		return spec.TargetInstance{}, &UnresolvedError{Kind: "target", Name: sv.TypeName}
	}
	cfg := tt.NewConfig()
	if err := mapFields(sv.Fields, cfg, lc); err != nil {
		return spec.TargetInstance{}, err
	}
	return spec.TargetInstance{
		Type:   tt,
		Config: cfg,
	}, nil
}

func linkDeploy(bv *eval.BlockResultVal, reg Registry, lc *linkConfig) (spec.DeployBlock, error) {
	db := spec.DeployBlock{
		Hooks: make(map[string][]spec.StepInstance),
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
	var extractHooks func(sv *eval.StructVal, si *spec.StepInstance) error
	extractHooks = func(sv *eval.StructVal, si *spec.StepInstance) error {
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
				db.Hooks[hid] = []spec.StepInstance{hookStep}
			}
			si.OnChange = append(si.OnChange, hid)
		}
		return nil
	}

	for _, v := range bv.Body {
		sv, ok := v.(*eval.StructVal)
		if !ok {
			continue
		}
		si, err := linkStep(sv, reg, lc)
		if err != nil {
			return db, err
		}
		if err := extractHooks(sv, &si); err != nil {
			return db, err
		}
		db.Steps = append(db.Steps, si)
	}

	return db, nil
}

func linkStep(sv *eval.StructVal, reg Registry, lc *linkConfig) (spec.StepInstance, error) {
	// Try qualified name first (e.g. "container.instance"), then leaf.
	name := sv.QualName
	st, ok := reg.StepType(name)
	if !ok {
		name = sv.TypeName
		st, ok = reg.StepType(name)
	}
	if !ok {
		return spec.StepInstance{}, &UnresolvedError{Kind: "step", Name: sv.TypeName}
	}
	cfg := st.NewConfig()
	if err := mapFields(sv.Fields, cfg, lc); err != nil {
		return spec.StepInstance{}, err
	}

	var desc string
	if d, ok := sv.Fields["desc"].(*eval.StringVal); ok {
		desc = d.V
	}

	return spec.StepInstance{
		Type:   st,
		Config: cfg,
		Desc:   desc,
	}, nil
}
