// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"testing"

	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/linker"
	"scampi.dev/scampi/std"
)

// TestStubsMatchGoConfigs is the drift lint that replaces the deleted
// stub generator (#163). Stubs in std/ are now hand-written and serve as
// the source of truth for parameter names, types, and validation
// attributes. This lint asserts the cheaper half of that contract:
// every step:-tagged Go field on a registered StepType/TargetType
// config struct must have a corresponding parameter on the matching
// stub decl, using the same snake_case conversion the linker applies
// at link time (linker.ToSnake).
//
// What this catches:
//
//   - Adding a new field to a Go config struct without updating the
//     stub. The field would silently default at link time and the LSP
//     would not surface it.
//   - Renaming a Go field without updating the stub.
//
// What this does NOT catch (deliberately):
//
//   - Stub params with no Go counterpart (synthetic fields the linker
//     handles itself, like on_change, are intentional).
//   - Type mismatches between Go field type and stub param type.
//     Trust mapFields' runtime conversion for now; revisit if drift
//     bites us in practice.
//   - Attribute correctness on stub params. That's the linker's job
//     at link time.
func TestStubsMatchGoConfigs(t *testing.T) {
	stubDecls := loadStubDecls(t)

	reg := engine.NewRegistry()

	type kindCfg struct {
		kind   string
		cfg    any
		isStep bool
	}
	var all []kindCfg
	for _, st := range reg.StepTypes() {
		all = append(all, kindCfg{kind: st.Kind(), cfg: st.NewConfig(), isStep: true})
	}
	for _, tt := range reg.TargetTypes() {
		all = append(all, kindCfg{kind: tt.Kind(), cfg: tt.NewConfig()})
	}

	for _, kc := range all {
		t.Run(kc.kind, func(t *testing.T) {
			params, found := lookupStubDecl(stubDecls, kc.kind, kc.isStep)
			if !found {
				t.Fatalf("no stub decl found for kind %q "+
					"(tried leaf, dotted module/leaf, and module-scoped target fallback)",
					kc.kind)
			}

			missing := missingStubParams(kc.cfg, params)
			if len(missing) == 0 {
				return
			}

			sort.Strings(missing)
			t.Errorf("stub for kind %q is missing parameters for Go fields:\n"+
				"  %s\n\n"+
				"stub params: %s\n"+
				"add these params to the matching decl in std/.../*.scampi",
				kc.kind,
				strings.Join(missing, "\n  "),
				strings.Join(sortedKeys(params), ", "))
		})
	}
}

// lookupStubDecl resolves a registry kind to a stub decl's parameter
// set. Mirrors linker/linker.go's resolution chain for steps and
// targets:
//
//   - dotted kind (`container.instance`, `rest.request`) → look up
//     leaf decl ("instance"/"request") in the matching module file
//   - undotted kind (`copy`, `pkg`, `ssh`, `local`) → search every
//     module for a leaf decl with that name; first hit wins. This is
//     how posix targets resolve: `decl ssh(...)` and `decl local(...)`
//     live in `posix.scampi` because the module hosts multiple targets
//     and they need distinct leaf identities.
//   - undotted target kind matching a module name (`rest`) → look for
//     `decl target` inside that module. The single-target module
//     pattern: when a module hosts exactly one target, calling it
//     `decl target(...)` makes the user-side syntax `rest.target { ... }`
//     read cleaner than `rest.rest { ... }` would. Both naming
//     conventions are legitimate; the linker juggles both via the
//     three-tier lookup in linker.linkTarget.
func lookupStubDecl(decls map[string]map[string]map[string]bool, kind string, isStep bool) (map[string]bool, bool) {
	if i := strings.IndexByte(kind, '.'); i >= 0 {
		mod, leaf := kind[:i], kind[i+1:]
		if modDecls, ok := decls[mod]; ok {
			if params, ok := modDecls[leaf]; ok {
				return params, true
			}
		}
		return nil, false
	}
	// Undotted kind: search every module for a matching leaf.
	for _, modDecls := range decls {
		if params, ok := modDecls[kind]; ok {
			return params, true
		}
	}
	// Target fallback: undotted target kind whose stub lives as
	// `decl target` in a module of the same name.
	if !isStep {
		if modDecls, ok := decls[kind]; ok {
			if params, ok := modDecls["target"]; ok {
				return params, true
			}
		}
	}
	return nil, false
}

// loadStubDecls walks std/*.scampi and std/*/*.scampi via the embed
// FS, parses each, and returns a nested map of module name → leaf
// decl name → set of parameter names. The module name comes from the
// stub's `module foo` header; the leaf is the last segment of the
// dotted decl name.
func loadStubDecls(t *testing.T) map[string]map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]map[string]bool{}

	err := fs.WalkDir(std.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".scampi") {
			return nil
		}
		src, err := std.FS.ReadFile(path)
		if err != nil {
			return err
		}
		l := lex.New(path, src)
		p := parse.New(l)
		file := p.Parse()
		if errs := p.Errors(); len(errs) > 0 {
			t.Fatalf("parse %s: %v", path, errs[0])
		}
		if errs := l.Errors(); len(errs) > 0 {
			t.Fatalf("lex %s: %v", path, errs[0])
		}
		mod := ""
		if file.Module != nil && file.Module.Name != nil {
			mod = file.Module.Name.Name
		}
		if _, ok := out[mod]; !ok {
			out[mod] = map[string]map[string]bool{}
		}
		for _, decl := range file.Decls {
			dd, ok := decl.(*ast.DeclDecl)
			if !ok {
				continue
			}
			leaf := leafDeclName(dd.Name)
			if leaf == "" {
				continue
			}
			if _, dup := out[mod][leaf]; dup {
				t.Fatalf("duplicate decl %q in module %q (saw it again in %s)",
					leaf, mod, path)
			}
			params := map[string]bool{}
			for _, f := range dd.Params {
				if f.Name != nil {
					params[f.Name.Name] = true
				}
			}
			out[mod][leaf] = params
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk std FS: %v", err)
	}
	return out
}

// missingStubParams returns the snake-cased names of every step:-tagged
// exported Go field that has no corresponding entry in stubParams.
func missingStubParams(cfg any, stubParams map[string]bool) []string {
	v := reflect.ValueOf(cfg)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()

	var missing []string
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Tag.Get("step") == "" {
			// Untagged exported fields are internal — not part
			// of the user-facing schema.
			continue
		}
		name := linker.ToSnake(f.Name)
		// Mirror the linker's keyword rename in mapFields.
		if name == "type" {
			name = "fs_type"
		}
		if !stubParams[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// leafDeclName returns the last segment of a stub decl's dotted name.
// posix-style decls like `decl pkg(...)` resolve to "pkg"; module-prefixed
// decls like `decl rest.request(...)` resolve to "request".
func leafDeclName(d *ast.DottedName) string {
	if d == nil || len(d.Parts) == 0 {
		return ""
	}
	return d.Parts[len(d.Parts)-1].Name
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
