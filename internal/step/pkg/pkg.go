// SPDX-License-Identifier: GPL-3.0-only

package pkg

import (
	"crypto/sha256"
	"encoding/hex"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
	"scampi.dev/scampi/internal/step/sharedop"
)

// keyCachePath returns the source-side cache path for a downloaded GPG key.
func keyCachePath(keyURL string) string {
	h := sha256.Sum256([]byte(keyURL))
	return ".scampi-cache/repo-keys/" + hex.EncodeToString(h[:16])
}

// State represents the desired package state.
type State uint8

const (
	StatePresent State = iota + 1
	StateAbsent
	StateLatest
)

const (
	statePresent = "present"
	stateAbsent  = "absent"
	stateLatest  = "latest"
)

func (s State) String() string {
	switch s {
	case StatePresent:
		return statePresent
	case StateAbsent:
		return stateAbsent
	case StateLatest:
		return stateLatest
	default:
		return "unknown"
	}
}

func parseState(s string) State {
	switch s {
	case statePresent:
		return StatePresent
	case stateAbsent:
		return StateAbsent
	case stateLatest:
		return StateLatest
	default:
		panic(errs.BUG("invalid pkg state %q - should have been caught by Validate", s))
	}
}

type (
	Pkg       struct{}
	PkgConfig struct {
		Desc     string
		Packages []string
		State    string
		Source   spec.PkgSourceRef
		Provides []string
		Requires []string
	}
	pkgStep struct {
		desc     string
		packages []string
		state    State
		source   spec.PkgSourceRef
		step     spec.DeclaredStep
	}
)

func (Pkg) Kind() string   { return "pkg" }
func (Pkg) NewConfig() any { return &PkgConfig{} }

func (c *PkgConfig) ResourceDeclarations() (provides, requires []string) {
	return c.Provides, c.Requires
}

func (p Pkg) Plan(step spec.DeclaredStep) (spec.Step, error) {
	cfg, ok := step.Config.(*PkgConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &PkgConfig{}, step.Config)
	}

	return &pkgStep{
		desc:     cfg.Desc,
		packages: cfg.Packages,
		state:    parseState(cfg.State),
		source:   cfg.Source,
		step:     step,
	}, nil
}

func (a *pkgStep) Desc() string { return a.desc }
func (a *pkgStep) Kind() string { return "pkg" }

func (a *pkgStep) Ops() []spec.Op {
	pkgsSource := a.step.Fields["packages"].Value

	// Build the package install/remove op.
	var pkgOp spec.Op
	if a.state == StateLatest {
		o := &ensureLatestPkgOp{packages: a.packages, pkgsSource: pkgsSource}
		o.SetStep(a)
		pkgOp = o
	} else {
		o := &ensurePkgOp{packages: a.packages, state: a.state, pkgsSource: pkgsSource}
		o.SetStep(a)
		pkgOp = o
	}

	if a.source.Kind == spec.PkgSourceNative {
		return []spec.Op{pkgOp}
	}

	// Third-party source - build the repo setup DAG:
	//   download key -> install key -> write repo config -> install packages
	var ops []spec.Op
	var lastDep spec.Op

	if a.source.KeyURL != "" {
		dlOp := &sharedop.DownloadOp{
			URL:       a.source.KeyURL,
			CachePath: keyCachePath(a.source.KeyURL),
		}
		dlOp.SetStep(a)
		ops = append(ops, dlOp)

		keyOp := &installKeyOp{source: a.source}
		keyOp.SetStep(a)
		keyOp.AddDependency(dlOp)
		ops = append(ops, keyOp)
		lastDep = keyOp
	}

	cfgOp := &writeRepoConfigOp{source: a.source}
	cfgOp.SetStep(a)
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
