// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"os"
	"path/filepath"

	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/mod"
)

// LoadUserModules finds scampi.mod by walking up from cfgPath,
// parses each dependency, and adds the resulting scopes to modules.
// Errors from individual modules are silently skipped — the checker
// will emit "unknown module" for anything that failed to load, which
// is a better UX than aborting the whole pipeline on a broken dep.
//
// This is the production counterpart to lsp/eval.go:loadUserModules.
// Both use LoadModule for the actual lex+parse+check of each dep.
// LoadUserModules finds scampi.mod, parses each dependency, adds
// scopes to modules (keyed by the full require path), and returns
// the parsed ASTs so the evaluator can register their funcs/decls.
func LoadUserModules(cfgPath string, modules map[string]*check.Scope) []eval.UserModule {
	modFile := findModFile(cfgPath)
	if modFile == "" {
		return nil
	}
	data, err := os.ReadFile(modFile)
	if err != nil {
		return nil
	}
	m, err := mod.Parse(modFile, data)
	if err != nil {
		return nil
	}

	var userMods []eval.UserModule
	for _, dep := range m.Require {
		dir := depDir(m, &dep)
		depData, depPath := readModuleEntry(dir, lastPathSegment(dep.Path))
		if depData == nil {
			continue
		}
		f, fileScope, err := LoadModule(modules, depPath, depData)
		if err != nil || fileScope == nil || f == nil || f.Module == nil {
			continue
		}
		modName := f.Module.Name.Name
		// Register by leaf name for expression resolution
		// (adguard.dns_rewrite) and by full import path for
		// import validation. checkImport uses the full path;
		// resolveModuleMember uses the leaf.
		modules[modName] = fileScope
		modules[dep.Path] = fileScope
		userMods = append(userMods, eval.UserModule{
			Name:   f.Module.Name.Name,
			File:   f,
			Source: depData,
		})
	}
	return userMods
}

// findModFile walks up from cfgPath looking for a scampi.mod file.
func findModFile(cfgPath string) string {
	dir := filepath.Dir(cfgPath)
	for {
		candidate := filepath.Join(dir, "scampi.mod")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func depDir(m *mod.Module, dep *mod.Dependency) string {
	if dep.IsLocal() {
		dir := dep.Version
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(m.Filename), dir)
		}
		return dir
	}
	return filepath.Join(mod.DefaultCacheDir(), dep.Path+"@"+dep.Version)
}

func lastPathSegment(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// readModuleEntry finds the entry point .scampi file in a module
// directory, trying _index.scampi then <name>.scampi.
func readModuleEntry(dir, name string) ([]byte, string) {
	for _, candidate := range []string{
		filepath.Join(dir, "_index.scampi"),
		filepath.Join(dir, name+".scampi"),
	} {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data, candidate
		}
	}
	return nil, ""
}
