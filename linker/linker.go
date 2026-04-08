// SPDX-License-Identifier: GPL-3.0-only

// Package linker bridges the scampi-lang frontend and the engine.
// It takes the evaluator's generic Result (StructVals, BlockResultVals)
// and resolves them against the engine registry to produce spec.Config.
package linker

import (
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/spec"
)

// Registry provides step and target type lookups. Implemented by
// engine.Registry — defined as an interface here to avoid a cycle.
type Registry interface {
	StepType(kind string) (spec.StepType, bool)
	TargetType(kind string) (spec.TargetType, bool)
}

// Link converts a lang eval result into a spec.Config by walking all
// values, interpreting them based on RetType/TypeName, and resolving
// step and target names against the engine registry.
func Link(result *eval.Result, reg Registry, path string) (spec.Config, error) {
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
			linkErr = linkStructVal(sv, result, reg, &cfg)
			return false // don't descend into fields
		case *eval.BlockResultVal:
			linkErr = linkBlockResult(sv, reg, &cfg)
			return false
		}
		return true
	})

	return cfg, linkErr
}

func linkStructVal(sv *eval.StructVal, r *eval.Result, reg Registry, cfg *spec.Config) error {
	switch sv.RetType {
	case "Target":
		ti, err := linkTarget(sv, reg)
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

func linkBlockResult(bv *eval.BlockResultVal, reg Registry, cfg *spec.Config) error {
	switch bv.TypeName {
	case "Deploy":
		db, err := linkDeploy(bv, reg)
		if err != nil {
			return err
		}
		cfg.Deploy = append(cfg.Deploy, db)
	}
	return nil
}

func linkTarget(sv *eval.StructVal, reg Registry) (spec.TargetInstance, error) {
	tt, ok := reg.TargetType(sv.TypeName)
	if !ok {
		return spec.TargetInstance{}, &UnresolvedError{Kind: "target", Name: sv.TypeName}
	}
	cfg := tt.NewConfig()
	if err := mapFields(sv.Fields, cfg); err != nil {
		return spec.TargetInstance{}, err
	}
	return spec.TargetInstance{
		Type:   tt,
		Config: cfg,
	}, nil
}

func linkDeploy(bv *eval.BlockResultVal, reg Registry) (spec.DeployBlock, error) {
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

	for _, v := range bv.Body {
		sv, ok := v.(*eval.StructVal)
		if !ok {
			continue
		}
		si, err := linkStep(sv, reg)
		if err != nil {
			return db, err
		}
		db.Steps = append(db.Steps, si)
	}

	return db, nil
}

func linkStep(sv *eval.StructVal, reg Registry) (spec.StepInstance, error) {
	st, ok := reg.StepType(sv.TypeName)
	if !ok {
		return spec.StepInstance{}, &UnresolvedError{Kind: "step", Name: sv.TypeName}
	}
	cfg := st.NewConfig()
	if err := mapFields(sv.Fields, cfg); err != nil {
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
