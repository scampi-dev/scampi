// SPDX-License-Identifier: GPL-3.0-only

package sysctl

import (
	"strings"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/spec"
)

type (
	Sysctl       struct{}
	SysctlConfig struct {
		_ struct{} `summary:"Manage kernel parameters via sysctl with optional persistence"`

		Desc     string   `step:"Human-readable description" optional:"true"`
		Key      string   `step:"Sysctl parameter name" example:"net.ipv4.ip_forward"`
		Value    string   `step:"Desired parameter value" example:"1"`
		Persist  bool     `step:"Write to /etc/sysctl.d/ for persistence across reboots" default:"true"`
		Promises []string `step:"Cross-deploy resources this step produces" optional:"true"`
		Inputs   []string `step:"Cross-deploy resources this step consumes" optional:"true"`
	}
	sysctlStep struct {
		desc    string
		key     string
		value   string
		persist bool
		step    spec.DeclaredStep
	}
)

func (Sysctl) Kind() string   { return "sysctl" }
func (Sysctl) NewConfig() any { return &SysctlConfig{} }

func (c *SysctlConfig) ResourceDeclarations() (promises, inputs []string) {
	return c.Promises, c.Inputs
}

func (Sysctl) Plan(step spec.DeclaredStep) (spec.Step, error) {
	cfg, ok := step.Config.(*SysctlConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &SysctlConfig{}, step.Config)
	}

	return &sysctlStep{
		desc:    cfg.Desc,
		key:     cfg.Key,
		value:   cfg.Value,
		persist: cfg.Persist,
		step:    step,
	}, nil
}

func (a *sysctlStep) Desc() string { return a.desc }
func (a *sysctlStep) Kind() string { return "sysctl" }

func (a *sysctlStep) Ops() []spec.Op {
	set := &setSysctlOp{
		key:   a.key,
		value: a.value,
	}
	set.SetStep(a)

	if !a.persist {
		cleanup := &cleanupSysctlOp{
			path: dropInPath(a.key),
		}
		cleanup.SetStep(a)
		cleanup.AddDependency(set)
		return []spec.Op{set, cleanup}
	}

	persist := &persistSysctlOp{
		key:   a.key,
		value: a.value,
		path:  dropInPath(a.key),
	}
	persist.SetStep(a)
	persist.AddDependency(set)

	return []spec.Op{set, persist}
}

// dropInPath returns the sysctl.d drop-in file path for a given key.
// Dots are replaced with dashes: net.ipv4.ip_forward -> 99-scampi-net-ipv4-ip_forward.conf
func dropInPath(key string) string {
	return "/etc/sysctl.d/99-scampi-" + strings.ReplaceAll(key, ".", "-") + ".conf"
}
