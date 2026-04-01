// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"testing"
)

func TestCatalogHasAllStepTypes(t *testing.T) {
	c := NewCatalog()

	steps := []string{
		"copy", "dir", "firewall", "group", "mount", "pkg",
		"run", "service", "symlink", "sysctl", "template",
		"unarchive", "user", "container.instance",
		"rest.request", "rest.resource",
	}

	for _, name := range steps {
		f, ok := c.Lookup(name)
		if !ok {
			t.Errorf("missing step builtin: %s", name)
			continue
		}
		if f.Summary == "" {
			t.Errorf("step %s has empty summary", name)
		}
		if !f.IsStep {
			t.Errorf("step %s should have IsStep=true", name)
		}
		if len(f.Params) == 0 {
			t.Errorf("step %s has no params", name)
		}
	}
}

func TestCatalogHasNonStepBuiltins(t *testing.T) {
	c := NewCatalog()

	names := []string{
		"deploy", "local", "inline", "remote", "system",
		"apt_repo", "dnf_repo", "ref", "env", "secret", "secrets",
		"target.local", "target.ssh", "target.rest",
		"rest.no_auth", "rest.basic", "rest.bearer", "rest.header",
		"rest.status", "rest.jq",
		"rest.tls.secure", "rest.tls.insecure", "rest.tls.ca_cert",
		"rest.body.json", "rest.body.string",
		"container.healthcheck.cmd",
	}

	for _, name := range names {
		if _, ok := c.Lookup(name); !ok {
			t.Errorf("missing non-step builtin: %s", name)
		}
	}
}

func TestCatalogModules(t *testing.T) {
	c := NewCatalog()

	modules := c.Modules()
	want := []string{"container", "rest", "target", "test"}
	if len(modules) != len(want) {
		t.Fatalf("got modules %v, want %v", modules, want)
	}
	for i, m := range modules {
		if m != want[i] {
			t.Errorf("module[%d] = %q, want %q", i, m, want[i])
		}
	}

	members := c.ModuleMembers("target")
	if len(members) != 3 {
		t.Fatalf("target members = %v, want 3 entries", members)
	}
}

func TestCatalogStepParamsHaveDescAndOnChange(t *testing.T) {
	c := NewCatalog()
	f, ok := c.Lookup("copy")
	if !ok {
		t.Fatal("missing copy builtin")
	}

	var hasDesc, hasOnChange bool
	for _, p := range f.Params {
		if p.Name == "desc" {
			hasDesc = true
		}
		if p.Name == "on_change" {
			hasOnChange = true
		}
	}
	if !hasDesc {
		t.Error("copy missing desc param")
	}
	if !hasOnChange {
		t.Error("copy missing on_change param")
	}
}

func TestCatalogNamesAreSorted(t *testing.T) {
	c := NewCatalog()
	names := c.Names()
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("names not sorted: %q before %q", names[i-1], names[i])
		}
	}
}
