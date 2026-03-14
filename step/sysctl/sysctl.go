// SPDX-License-Identifier: GPL-3.0-only

package sysctl

import (
	"strings"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
)

type (
	Sysctl       struct{}
	SysctlConfig struct {
		_ struct{} `summary:"Manage kernel parameters via sysctl with optional persistence"`

		Desc    string `step:"Human-readable description" optional:"true"`
		Key     string `step:"Sysctl parameter name" example:"net.ipv4.ip_forward"`
		Value   string `step:"Desired parameter value" example:"1"`
		Persist bool   `step:"Write to /etc/sysctl.d/ for persistence across reboots" default:"true"`
	}
	sysctlAction struct {
		idx     int
		desc    string
		key     string
		value   string
		persist bool
		step    spec.StepInstance
	}
)

func (Sysctl) Kind() string   { return "sysctl" }
func (Sysctl) NewConfig() any { return &SysctlConfig{} }

func (Sysctl) Plan(idx int, step spec.StepInstance) (spec.Action, error) {
	cfg, ok := step.Config.(*SysctlConfig)
	if !ok {
		return nil, errs.BUG("expected %T got %T", &SysctlConfig{}, step.Config)
	}

	return &sysctlAction{
		idx:     idx,
		desc:    cfg.Desc,
		key:     cfg.Key,
		value:   cfg.Value,
		persist: cfg.Persist,
		step:    step,
	}, nil
}

func (a *sysctlAction) Desc() string { return a.desc }
func (a *sysctlAction) Kind() string { return "sysctl" }

func (a *sysctlAction) Ops() []spec.Op {
	set := &setSysctlOp{
		key:   a.key,
		value: a.value,
	}
	set.SetAction(a)

	if !a.persist {
		cleanup := &cleanupSysctlOp{
			path: dropInPath(a.key),
		}
		cleanup.SetAction(a)
		cleanup.AddDependency(set)
		return []spec.Op{set, cleanup}
	}

	persist := &persistSysctlOp{
		key:   a.key,
		value: a.value,
		path:  dropInPath(a.key),
	}
	persist.SetAction(a)
	persist.AddDependency(set)

	return []spec.Op{set, persist}
}

// dropInPath returns the sysctl.d drop-in file path for a given key.
// Dots are replaced with dashes: net.ipv4.ip_forward -> 99-scampi-net-ipv4-ip_forward.conf
func dropInPath(key string) string {
	return "/etc/sysctl.d/99-scampi-" + strings.ReplaceAll(key, ".", "-") + ".conf"
}
