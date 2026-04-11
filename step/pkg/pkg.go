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

// StateValues is the exhaustive list of accepted state strings.
var StateValues = []string{statePresent, stateAbsent, stateLatest}

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
		panic(errs.BUG("invalid pkg state %q — should have been caught by Validate", s))
	}
}

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
		desc     string
		packages []string
		state    State
		source   spec.PkgSourceRef
		step     spec.StepInstance
	}
)

func (*PkgConfig) FieldEnumValues() map[string][]string {
	return map[string][]string{
		"state": StateValues,
	}
}

func (Pkg) Kind() string   { return "pkg" }
func (Pkg) NewConfig() any { return &PkgConfig{} }

func (p Pkg) Plan(step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*PkgConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &PkgConfig{}, step.Config)
	}

	return &pkgAction{
		desc:     cfg.Desc,
		packages: cfg.Packages,
		state:    parseState(cfg.State),
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
