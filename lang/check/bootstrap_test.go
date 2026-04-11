// SPDX-License-Identifier: GPL-3.0-only

package check

import (
	"testing"
	"testing/fstest"
)

func TestBootstrapModules_CrossSubmoduleImport(t *testing.T) {
	fsys := fstest.MapFS{
		"std.scampi": {Data: []byte(
			"module std\n" +
				"type Step\n" +
				"type Target\n",
		)},
		"a/a.scampi": {Data: []byte(
			"module a\n" +
				"import \"std\"\n" +
				"type AVal\n",
		)},
		"b/b.scampi": {Data: []byte(
			"module b\n" +
				"import \"std\"\n" +
				"import \"std/a\"\n" +
				"type BVal\n",
		)},
	}

	modules, err := BootstrapModules(fsys)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	for _, want := range []string{"std", "a", "b"} {
		if _, ok := modules[want]; !ok {
			t.Errorf("module %q not registered; got %v", want, keys(modules))
		}
	}
}

func TestBootstrapModules_CycleDetection(t *testing.T) {
	fsys := fstest.MapFS{
		"std.scampi": {Data: []byte("module std\n")},
		"a/a.scampi": {Data: []byte(
			"module a\n" +
				"import \"std/b\"\n" +
				"type AVal\n",
		)},
		"b/b.scampi": {Data: []byte(
			"module b\n" +
				"import \"std/a\"\n" +
				"type BVal\n",
		)},
	}

	_, err := BootstrapModules(fsys)
	if err == nil {
		t.Fatal("expected an error for circular imports, got nil")
	}
	t.Logf("got error: %v", err)
}

func keys(m map[string]*Scope) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

