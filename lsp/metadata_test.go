// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"testing"
)

func TestCatalogHasAllStepTypes(t *testing.T) {
	c := NewCatalog()

	steps := []string{
		"posix.copy", "posix.dir", "posix.firewall", "posix.group",
		"posix.mount", "posix.pkg", "posix.run", "posix.service",
		"posix.symlink", "posix.sysctl", "posix.template",
		"posix.unarchive", "posix.user",
		"container.instance",
		"rest.request", "rest.resource",
	}

	for _, name := range steps {
		f, ok := c.Lookup(name)
		if !ok {
			t.Errorf("missing step builtin: %s", name)
			continue
		}
		if !f.IsStep {
			t.Errorf("step %s should have IsStep=true", name)
		}
		if len(f.Params) == 0 {
			t.Errorf("step %s has no params", name)
		}
	}
}

func TestCatalogHasNonStepDecls(t *testing.T) {
	c := NewCatalog()

	names := []string{
		"std.deploy", "std.env", "std.secret", "std.secrets",
		"posix.local", "posix.ssh",
		"posix.source_local", "posix.source_inline", "posix.source_remote",
		"posix.pkg_system", "posix.pkg_apt_repo", "posix.pkg_dnf_repo",
		"rest.target",
		"rest.no_auth", "rest.basic", "rest.bearer", "rest.header",
		"rest.status", "rest.jq",
		"rest.tls_secure", "rest.tls_insecure", "rest.tls_ca_cert",
		"rest.body_json", "rest.body_string",
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
	want := []string{"container", "posix", "rest", "std", "test"}
	if len(modules) != len(want) {
		t.Fatalf("got modules %v, want %v", modules, want)
	}
	for i, m := range modules {
		if m != want[i] {
			t.Errorf("module[%d] = %q, want %q", i, m, want[i])
		}
	}

	members := c.ModuleMembers("posix")
	if len(members) == 0 {
		t.Fatal("posix should have members")
	}
}

func TestCatalogStepParamsHaveOnChange(t *testing.T) {
	c := NewCatalog()
	f, ok := c.Lookup("posix.copy")
	if !ok {
		t.Fatal("missing posix.copy builtin")
	}

	var hasOnChange bool
	for _, p := range f.Params {
		if p.Name == "on_change" {
			hasOnChange = true
		}
	}
	if !hasOnChange {
		t.Error("posix.copy missing on_change param")
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

func TestCatalogEnumValues(t *testing.T) {
	c := NewCatalog()
	f, ok := c.Lookup("posix.service")
	if !ok {
		t.Fatal("missing posix.service")
	}

	for _, p := range f.Params {
		if p.Name == "state" {
			if len(p.EnumValues) == 0 {
				t.Error("service state param should have enum values")
			}
			return
		}
	}
	t.Error("service should have a state param")
}
