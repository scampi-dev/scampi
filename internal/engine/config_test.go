// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"errors"
	"testing"

	"scampi.dev/scampi/internal/spec"
)

func testConfig(deploys []spec.DeclaredDeploy, targets map[string]spec.DeclaredTarget) spec.DeclaredConfig {
	return spec.DeclaredConfig{
		Path:    "/test/config.scampi",
		Deploy:  deploys,
		Targets: targets,
	}
}

// Resolve
// -----------------------------------------------------------------------------

func TestResolve_ExplicitDeployAndTarget(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "prod", Targets: []string{"server"}, Steps: []spec.DeclaredStep{{Desc: "s1"}}},
		},
		map[string]spec.DeclaredTarget{
			"server": {Config: "srv-cfg"},
		},
	)

	rc, err := Resolve(cfg, "prod", "server")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.DeployName != "prod" {
		t.Errorf("DeployName = %q, want %q", rc.DeployName, "prod")
	}
	if rc.TargetName != "server" {
		t.Errorf("TargetName = %q, want %q", rc.TargetName, "server")
	}
	if rc.Path != cfg.Path {
		t.Errorf("Path = %q, want %q", rc.Path, cfg.Path)
	}
	if len(rc.Steps) != 1 || rc.Steps[0].Desc != "s1" {
		t.Errorf("Steps not propagated correctly")
	}
}

func TestResolve_EmptyNames_PicksFirst(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "dev", Targets: []string{"laptop"}, Steps: []spec.DeclaredStep{{Desc: "s1"}}},
		},
		map[string]spec.DeclaredTarget{
			"laptop": {},
		},
	)

	rc, err := Resolve(cfg, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.DeployName != "dev" {
		t.Errorf("DeployName = %q, want %q", rc.DeployName, "dev")
	}
	if rc.TargetName != "laptop" {
		t.Errorf("TargetName = %q, want %q", rc.TargetName, "laptop")
	}
}

func TestResolve_UnknownDeployBlock(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "dev", Targets: []string{"laptop"}},
		},
		map[string]spec.DeclaredTarget{"laptop": {}},
	)

	_, err := Resolve(cfg, "staging", "")
	if err == nil {
		t.Fatal("expected error")
	}
	var target UnknownDeployBlockError
	if !errors.As(err, &target) {
		t.Errorf("expected UnknownDeployBlockError, got %T", err)
	}
}

func TestResolve_NoDeployBlocks(t *testing.T) {
	cfg := testConfig([]spec.DeclaredDeploy{}, nil)

	_, err := Resolve(cfg, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	var target NoDeployBlocksError
	if !errors.As(err, &target) {
		t.Errorf("expected NoDeployBlocksError, got %T", err)
	}
}

func TestResolve_NoTargetsInDeploy(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "dev", Targets: []string{}},
		},
		map[string]spec.DeclaredTarget{},
	)

	_, err := Resolve(cfg, "dev", "")
	if err == nil {
		t.Fatal("expected error")
	}
	var target NoTargetsInDeployError
	if !errors.As(err, &target) {
		t.Errorf("expected NoTargetsInDeployError, got %T", err)
	}
}

func TestResolve_UnknownTarget(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "dev", Targets: []string{"missing"}},
		},
		map[string]spec.DeclaredTarget{},
	)

	_, err := Resolve(cfg, "dev", "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var target UnknownTargetError
	if !errors.As(err, &target) {
		t.Errorf("expected UnknownTargetError, got %T", err)
	}
}

func TestResolve_TargetNotInDeploy(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "dev", Targets: []string{"laptop"}},
		},
		map[string]spec.DeclaredTarget{
			"laptop": {},
			"server": {},
		},
	)

	_, err := Resolve(cfg, "dev", "server")
	if err == nil {
		t.Fatal("expected error")
	}
	var target TargetNotInDeployError
	if !errors.As(err, &target) {
		t.Errorf("expected TargetNotInDeployError, got %T", err)
	}
}

// ResolveMultiple
// -----------------------------------------------------------------------------

func TestResolveMultiple_AllDeploys(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "prod", Targets: []string{"server"}},
			{Name: "dev", Targets: []string{"laptop"}},
		},
		map[string]spec.DeclaredTarget{
			"server": {},
			"laptop": {},
		},
	)

	results, err := ResolveMultiple(cfg, spec.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	names := map[string]bool{}
	for _, r := range results {
		names[r.DeployName+"/"+r.TargetName] = true
	}
	if !names["prod/server"] {
		t.Error("missing prod/server combination")
	}
	if !names["dev/laptop"] {
		t.Error("missing dev/laptop combination")
	}
}

func TestResolveMultiple_FilterByDeploy(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "prod", Targets: []string{"server"}},
			{Name: "dev", Targets: []string{"laptop"}},
		},
		map[string]spec.DeclaredTarget{
			"server": {},
			"laptop": {},
		},
	)

	results, err := ResolveMultiple(cfg, spec.ResolveOptions{DeployNames: []string{"prod"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].DeployName != "prod" {
		t.Errorf("expected deploy 'prod', got %q", results[0].DeployName)
	}
}

func TestResolveMultiple_FilterByTarget(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "prod", Targets: []string{"server", "backup"}},
		},
		map[string]spec.DeclaredTarget{
			"server": {},
			"backup": {},
		},
	)

	results, err := ResolveMultiple(cfg, spec.ResolveOptions{TargetNames: []string{"backup"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].TargetName != "backup" {
		t.Errorf("expected target 'backup', got %q", results[0].TargetName)
	}
}

func TestResolveMultiple_UnknownDeployFilter(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "dev", Targets: []string{"laptop"}},
		},
		map[string]spec.DeclaredTarget{"laptop": {}},
	)

	_, err := ResolveMultiple(cfg, spec.ResolveOptions{DeployNames: []string{"staging"}})
	if err == nil {
		t.Fatal("expected error")
	}
	var target UnknownDeployBlockError
	if !errors.As(err, &target) {
		t.Errorf("expected UnknownDeployBlockError, got %T", err)
	}
}

func TestResolveMultiple_TargetFilterMatchesNone(t *testing.T) {
	cfg := testConfig(
		[]spec.DeclaredDeploy{
			{Name: "dev", Targets: []string{"laptop"}},
		},
		map[string]spec.DeclaredTarget{"laptop": {}},
	)

	_, err := ResolveMultiple(cfg, spec.ResolveOptions{TargetNames: []string{"missing"}})
	if err == nil {
		t.Fatal("expected error")
	}
	var target NoDeployBlocksError
	if !errors.As(err, &target) {
		t.Errorf("expected NoDeployBlocksError, got %T", err)
	}
}

func TestResolveMultiple_NoDeployBlocks(t *testing.T) {
	cfg := testConfig([]spec.DeclaredDeploy{}, nil)

	_, err := ResolveMultiple(cfg, spec.ResolveOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	var target NoDeployBlocksError
	if !errors.As(err, &target) {
		t.Errorf("expected NoDeployBlocksError, got %T", err)
	}
}
