// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"os"
	"path/filepath"
	"strings"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
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
	return LoadUserModulesFromMod(m, modules)
}

// LoadUserModulesFromMod is the shared core used by both the linker
// (via LoadUserModules) and the LSP (which already has a parsed
// *mod.Module from its initialization). Loads all .scampi files
// in each dependency directory (Go package model) and returns the
// merged UserModules for the evaluator.
func LoadUserModulesFromMod(m *mod.Module, modules map[string]*check.Scope) []eval.UserModule {
	var userMods []eval.UserModule

	// Implicit self-registration: the module declared by scampi.mod
	// is always available by its own path — no self-require needed.
	// This mirrors Go where `import "github.com/foo/bar/sub"` works
	// within the bar module without requiring yourself.
	// Skip if the dir only has `module main` files — those are user
	// configs, not importable modules.
	if m.Module != "" {
		selfDir := filepath.Dir(m.Filename)
		selfFiles := readModuleDir(selfDir)
		if len(selfFiles) > 0 {
			if um := loadMultiFileModule(selfFiles, modules); um != nil && um.Name != "main" {
				modules[um.Name] = um.scope
				modules[m.Module] = um.scope
				userMods = append(userMods, um.UserModule)
			}
		}
	}

	// Local subdirectory modules: scan all subdirs under the module
	// root for .scampi files. Each subdir that produces a valid
	// non-main module becomes importable by its full path
	// (<module-path>/<subdir>). This is the Go package convention —
	// no require entry needed for directories in your own module.
	if m.Module != "" {
		modRoot := filepath.Dir(m.Filename)
		subMods := loadLocalSubmodules(modRoot, m.Module, modules)
		userMods = append(userMods, subMods...)
	}

	for _, dep := range m.Require {
		// Skip self-references if someone still has one.
		if dep.Path == m.Module {
			continue
		}
		dir := depDir(m, &dep)

		files := readModuleDir(dir)
		if len(files) == 0 {
			depData, depPath := readModuleEntry(dir, lastPathSegment(dep.Path))
			if depData == nil {
				continue
			}
			files = []moduleFile{{Path: depPath, Data: depData}}
		}

		um := loadMultiFileModule(files, modules)
		if um == nil {
			continue
		}
		pub := um.scope.PublicView()
		modules[um.Name] = pub
		modules[dep.Path] = pub
		userMods = append(userMods, um.UserModule)
	}
	return userMods
}

// loadLocalSubmodules scans subdirectories under modRoot for .scampi
// files, loads each as a module, and registers them under their full
// import path (<modPath>/<relDir>).
func loadLocalSubmodules(
	modRoot string,
	modPath string,
	modules map[string]*check.Scope,
) []eval.UserModule {
	var userMods []eval.UserModule
	_ = filepath.WalkDir(modRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path == modRoot {
			return nil
		}
		base := d.Name()
		if strings.HasPrefix(base, ".") || base == "vendor" {
			return filepath.SkipDir
		}
		files := readModuleDir(path)
		if len(files) == 0 {
			return nil
		}
		um := loadMultiFileModule(files, modules)
		if um == nil || um.Name == "main" {
			return nil
		}
		rel, _ := filepath.Rel(modRoot, path)
		importPath := modPath + "/" + filepath.ToSlash(rel)
		pub := um.scope.PublicView()
		modules[um.Name] = pub
		modules[importPath] = pub
		userMods = append(userMods, um.UserModule)
		return nil
	})
	return userMods
}

type loadedModule struct {
	eval.UserModule
	scope *check.Scope
}

// loadMultiFileModule parses all .scampi files in the module
// directory using a fixed-point approach: try to check each file
// against the accumulated module map, and retry files that failed
// until no progress is made. This handles cross-file dependencies
// (e.g. _index.scampi referencing functions from api.scampi)
// regardless of file ordering.
func loadMultiFileModule(
	files []moduleFile,
	stdModules map[string]*check.Scope,
) *loadedModule {
	// Parse all files first.
	type parsed struct {
		file *ast.File
		mf   moduleFile
	}
	var allParsed []parsed
	for _, mf := range files {
		l := lex.New(mf.Path, mf.Data)
		p := parse.New(l)
		f := p.Parse()
		if f == nil || f.Module == nil {
			continue
		}
		if l.Errors() != nil || p.Errors() != nil {
			continue
		}
		allParsed = append(allParsed, parsed{file: f, mf: mf})
	}
	if len(allParsed) == 0 {
		return nil
	}

	modName := allParsed[0].file.Module.Name.Name

	// Build a merged module map: std modules + a fresh scope for
	// this module. All files' forward-declarations go into the
	// module scope so cross-file references resolve.
	modScope := check.NewScope(nil, check.ScopeFile)
	mergedModules := make(map[string]*check.Scope, len(stdModules)+1)
	for k, v := range stdModules {
		mergedModules[k] = v
	}
	mergedModules[modName] = modScope

	// Phase 1: register all forward declarations from all files
	// into the shared scope. This makes every func/decl/type
	// visible to every file before body-checking runs — the Go
	// package model where all files in a dir share one namespace.
	for _, p := range allParsed {
		c := check.New(mergedModules)
		c.WithScope(modScope)
		c.RegisterForwardDecls(p.file)
	}

	// Phase 2: full check of each file's body against the shared
	// scope. All forward declarations are already registered, so
	// cross-file references resolve. Errors are tolerated.
	var mergedFile ast.File
	mergedFile.Module = allParsed[0].file.Module
	for _, p := range allParsed {
		c := check.New(mergedModules)
		c.WithScope(modScope)
		c.Check(p.file)
		mergedFile.Decls = append(mergedFile.Decls, p.file.Decls...)
	}

	return &loadedModule{
		UserModule: eval.UserModule{
			Name:   modName,
			File:   &mergedFile,
			Source: allParsed[0].mf.Data,
		},
		scope: modScope,
	}
}

// loadSiblingDecls finds sibling .scampi files in the same directory
// that declare the same module name. Returns a pre-populated scope
// with their forward declarations so the caller's Check sees them.
// Returns nil if there are no siblings (single-file module).
// brokenSibling records a sibling file that was skipped due to errors.
type brokenSibling struct {
	path     string
	firstErr string
}

func loadSiblingDecls(
	cfgPath string,
	modName string,
	modules map[string]*check.Scope,
) (*check.Scope, []brokenSibling, error) {
	dir := filepath.Dir(cfgPath)
	base := filepath.Base(cfgPath)
	siblings := readModuleDir(dir)
	if len(siblings) == 0 {
		return nil, nil, nil
	}

	var broken []brokenSibling
	scope := check.NewScope(nil, check.ScopeFile)
	for _, mf := range siblings {
		if filepath.Base(mf.Path) == base {
			continue // skip the file being checked — Check will add its own decls
		}
		l := lex.New(mf.Path, mf.Data)
		p := parse.New(l)
		f := p.Parse()
		if f == nil || f.Module == nil || f.Module.Name.Name != modName {
			continue
		}
		if errs := l.Errors(); len(errs) > 0 {
			broken = append(broken, brokenSibling{path: mf.Path, firstErr: errs[0].Error()})
			continue
		}
		if errs := p.Errors(); len(errs) > 0 {
			broken = append(broken, brokenSibling{path: mf.Path, firstErr: errs[0].Error()})
			continue
		}
		c := check.New(modules)
		c.WithScope(scope)
		c.RegisterForwardDecls(f)
		if cErrs := c.Errors(); len(cErrs) > 0 {
			return nil, nil, wrapLangErrors(cErrs, mf.Path, mf.Data)
		}
	}
	return scope, broken, nil
}

// loadSiblingUserModules builds eval.UserModule entries for sibling
// files in the same directory that declare the same module name. This
// gives the evaluator access to non-pub functions defined in sibling
// files — the eval-layer counterpart of loadSiblingDecls (which only
// feeds the type checker).
func loadSiblingUserModules(
	cfgPath string,
	modName string,
	modules map[string]*check.Scope,
) ([]eval.UserModule, []brokenSibling) {
	dir := filepath.Dir(cfgPath)
	base := filepath.Base(cfgPath)
	siblings := readModuleDir(dir)

	var result []eval.UserModule
	var broken []brokenSibling
	for _, mf := range siblings {
		if filepath.Base(mf.Path) == base {
			continue
		}
		l := lex.New(mf.Path, mf.Data)
		p := parse.New(l)
		f := p.Parse()
		if f == nil || f.Module == nil || f.Module.Name.Name != modName {
			continue
		}
		if errs := l.Errors(); len(errs) > 0 {
			broken = append(broken, brokenSibling{path: mf.Path, firstErr: errs[0].Error()})
			continue
		}
		if errs := p.Errors(); len(errs) > 0 {
			broken = append(broken, brokenSibling{path: mf.Path, firstErr: errs[0].Error()})
			continue
		}
		c := check.New(modules)
		c.Check(f)
		if cErrs := c.Errors(); len(cErrs) > 0 {
			broken = append(broken, brokenSibling{path: mf.Path, firstErr: cErrs[0].Msg})
			continue
		}
		result = append(result, eval.UserModule{
			Name:   modName,
			File:   f,
			Source: mf.Data,
		})
	}
	return result, broken
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
	cacheDir := filepath.Join(mod.DefaultCacheDir(), dep.Path+"@"+dep.Version)
	_ = ensureRemoteDep(dep.Path, dep.Version, cacheDir)
	return cacheDir
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

// moduleFile pairs a parsed file path with its raw source bytes.
type moduleFile struct {
	Path string
	Data []byte
}

// readModuleDir reads ALL .scampi files in a module directory.
// A module = a directory: all .scampi files contribute declarations
// to the same module scope (like Go packages). Returns the files
// sorted by name for deterministic ordering.
func readModuleDir(dir string) []moduleFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []moduleFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".scampi") {
			continue
		}
		// Skip test files — they're not part of the module's
		// public API.
		if strings.HasSuffix(e.Name(), "_test.scampi") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		files = append(files, moduleFile{Path: path, Data: data})
	}
	return files
}
