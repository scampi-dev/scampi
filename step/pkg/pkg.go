// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"crypto/sha256"
	"encoding/hex"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
)

// keyCachePath returns the source-side cache path for a downloaded GPG key.
func keyCachePath(keyURL string) string {
	h := sha256.Sum256([]byte(keyURL))
	return ".scampi-cache/repo-keys/" + hex.EncodeToString(h[:16])
}

// Desired package state values.
const (
	StatePresent = "present"
	StateAbsent  = "absent"
	StateLatest  = "latest"
)

type (
	Pkg       struct{}
	PkgConfig struct {
		_ struct{} `summary:"Ensure packages are present, absent, or at the latest version on the target"`

		Desc     string            `step:"Human-readable description" optional:"true"`
		Packages []string          `step:"Packages to manage" example:"[\"nginx\", \"curl\"]"`
		State    string            `step:"Desired package state" default:"present" example:"latest"`
		Source   spec.PkgSourceRef `step:"Package source" example:"system()|apt_repo(url=..., key_url=...)"`
	}
	pkgAction struct {
		idx      int
		desc     string
		packages []string
		state    string
		source   spec.PkgSourceRef
		step     spec.StepInstance
	}
)

func (Pkg) Kind() string   { return "pkg" }
func (Pkg) NewConfig() any { return &PkgConfig{} }

func (c *PkgConfig) Validate(step spec.StepInstance) error {
	if len(c.Packages) == 0 {
		return EmptyPackagesError{
			Source: step.Fields["packages"].Value,
		}
	}
	switch c.State {
	case StatePresent, StateAbsent, StateLatest:
	default:
		return InvalidStateError{
			Got:     c.State,
			Allowed: []string{StatePresent, StateAbsent, StateLatest},
			Source:  step.Fields["state"].Value,
		}
	}
	return nil
}

func (p Pkg) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*PkgConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &PkgConfig{}, step.Config)
	}

	if err := cfg.Validate(step); err != nil {
		return nil, err
	}

	return &pkgAction{
		idx:      idx,
		desc:     cfg.Desc,
		packages: cfg.Packages,
		state:    cfg.State,
		source:   cfg.Source,
		step:     step,
	}, nil
}

func (a *pkgAction) Desc() string { return a.desc }
func (a *pkgAction) Kind() string { return "pkg" }

func (a *pkgAction) Ops() []spec.Op {
	pkgsSource := a.step.Fields["packages"].Value

	// Build the package install/remove op.
	var pkgOp spec.Op
	if a.state == StateLatest {
		o := &ensureLatestPkgOp{packages: a.packages, pkgsSource: pkgsSource}
		o.SetAction(a)
		pkgOp = o
	} else {
		o := &ensurePkgOp{packages: a.packages, state: a.state, pkgsSource: pkgsSource}
		o.SetAction(a)
		pkgOp = o
	}

	if a.source.Kind == spec.PkgSourceNative {
		return []spec.Op{pkgOp}
	}

	// Third-party source — build the repo setup DAG:
	//   download key → install key → write repo config → install packages
	var ops []spec.Op
	var lastDep spec.Op

	if a.source.KeyURL != "" {
		dlOp := &sharedops.DownloadOp{
			URL:       a.source.KeyURL,
			CachePath: keyCachePath(a.source.KeyURL),
		}
		dlOp.SetAction(a)
		ops = append(ops, dlOp)

		keyOp := &installKeyOp{source: a.source}
		keyOp.SetAction(a)
		keyOp.AddDependency(dlOp)
		ops = append(ops, keyOp)
		lastDep = keyOp
	}

	cfgOp := &writeRepoConfigOp{source: a.source}
	cfgOp.SetAction(a)
	if lastDep != nil {
		cfgOp.AddDependency(lastDep)
	}
	ops = append(ops, cfgOp)

	// Package op depends on repo config being written.
	switch o := pkgOp.(type) {
	case *ensurePkgOp:
		o.AddDependency(cfgOp)
	case *ensureLatestPkgOp:
		o.AddDependency(cfgOp)
	}
	ops = append(ops, pkgOp)

	return ops
}
