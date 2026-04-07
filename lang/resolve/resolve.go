// SPDX-License-Identifier: GPL-3.0-only

// Package resolve maps import paths to parsed and type-checked modules.
// It reads source files through fs.FS (no os dependency) and caches
// resolved modules for the lifetime of the resolver.
package resolve

import (
	"errors"
	"io/fs"
	"strings"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
)

// Sentinel errors
// -----------------------------------------------------------------------------

var (
	ErrModuleNotFound = errors.New("module not found")
	ErrNoDependency   = errors.New("no matching dependency")
	ErrNoCache        = errors.New("no cache configured")
	ErrNoFS           = errors.New("no filesystem configured")
	ErrNoFiles        = errors.New("no .scampi files in directory")
)

// Dependency is a single entry from the merged require/replace table.
// The caller (mod2/ or test harness) parses scampi.mod and provides
// these; the resolver never touches the manifest format.
type Dependency struct {
	Path      string // canonical import path
	Version   string // semver (remote) or empty
	LocalPath string // relative dir (from replace), empty for remote
}

// IsLocal reports whether this dependency resolves from the project
// root rather than the module cache.
func (d Dependency) IsLocal() bool { return d.LocalPath != "" }

// Config configures a Resolver.
type Config struct {
	// ModulePath is this project's module path (from scampi.mod).
	ModulePath string

	// RootFS is the project root. Intra-project files and local
	// deps resolve from here.
	RootFS fs.FS

	// CacheFS is the module cache. Remote deps resolve from here.
	// May be nil if all deps are local.
	CacheFS fs.FS

	// Deps is the merged require/replace table.
	Deps []Dependency

	// StdModules maps leaf names ("std", "target") to pre-built
	// scopes. These are never loaded from the filesystem.
	StdModules map[string]*check.Scope
}

// Module is a resolved import — its parsed AST and exported scope.
type Module struct {
	Path  string
	File  *ast.File
	Scope *check.Scope
}

// Resolver maps import paths to checked modules. Safe for use within
// a single compilation; not safe for concurrent use.
type Resolver struct {
	cfg    Config
	loaded map[string]*Module
	errs   []Error
}

// Error is a resolution error surfaced to the caller (accumulated on
// the resolver). Contains the import path and a human-readable message.
type Error struct {
	ImportPath string
	Msg        string
}

func (e Error) Error() string {
	return "resolve " + e.ImportPath + ": " + e.Msg
}

// ResolveError is a typed internal error wrapping a sentinel. Callers
// can use errors.Is(err, ErrModuleNotFound) etc.
type ResolveError struct {
	Path string
	Err  error
}

func (e *ResolveError) Error() string { return e.Err.Error() + ": " + e.Path }
func (e *ResolveError) Unwrap() error { return e.Err }

// New creates a resolver from the given config.
func New(cfg Config) *Resolver {
	return &Resolver{
		cfg:    cfg,
		loaded: make(map[string]*Module),
	}
}

// Errors returns accumulated resolution errors.
func (r *Resolver) Errors() []Error { return r.errs }

// Resolve resolves an import path to a module. Returns nil on error
// (after recording a diagnostic). Results are cached.
func (r *Resolver) Resolve(importPath string) *Module {
	if m, ok := r.loaded[importPath]; ok {
		return m
	}

	// Std modules (hardcoded scopes, not loaded from FS).
	leaf := importLeaf(importPath)
	if scope, ok := r.cfg.StdModules[leaf]; ok {
		m := &Module{Path: importPath, Scope: scope}
		r.loaded[importPath] = m
		return m
	}

	// Collect source files (one for single-file module, multiple for directory).
	sources, err := r.readModule(importPath)
	if err != nil {
		r.errs = append(r.errs, Error{ImportPath: importPath, Msg: err.Error()})
		return nil
	}

	// Parse each file independently, merge scopes.
	scope := check.NewScope(nil, check.ScopeFile)
	var firstFile *ast.File
	for _, src := range sources {
		l := lex.New(importPath, src.data)
		p := parse.New(l)
		f := p.Parse()
		if errs := l.Errors(); len(errs) > 0 {
			r.errs = append(r.errs, Error{ImportPath: importPath, Msg: errs[0].Error()})
			return nil
		}
		if errs := p.Errors(); len(errs) > 0 {
			r.errs = append(r.errs, Error{ImportPath: importPath, Msg: errs[0].Error()})
			return nil
		}
		c := check.New()
		c.Check(f)
		if errs := c.Errors(); len(errs) > 0 {
			r.errs = append(r.errs, Error{ImportPath: importPath, Msg: errs[0].Error()})
		}
		r.mergeScope(scope, f)
		if firstFile == nil {
			firstFile = f
		}
	}

	m := &Module{Path: importPath, File: firstFile, Scope: scope}
	r.loaded[importPath] = m
	return m
}

func (r *Resolver) mergeScope(into *check.Scope, f *ast.File) {
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.StructDecl:
			into.Define(&check.Symbol{Name: d.Name.Name, Kind: check.SymStruct, Span: d.SrcSpan})
		case *ast.EnumDecl:
			into.Define(&check.Symbol{Name: d.Name.Name, Kind: check.SymEnum, Span: d.SrcSpan})
		case *ast.FuncDecl:
			into.Define(&check.Symbol{Name: d.Name.Name, Kind: check.SymFunc, Span: d.SrcSpan})
		case *ast.DeclDecl:
			into.Define(&check.Symbol{Name: d.Name.Parts[0].Name, Kind: check.SymDecl, Span: d.SrcSpan})
		case *ast.LetDecl:
			into.Define(&check.Symbol{Name: d.Name.Name, Kind: check.SymLet, Span: d.SrcSpan})
		}
	}
}

type sourceFile struct {
	name string
	data []byte
}

func (r *Resolver) readModule(importPath string) ([]sourceFile, error) {
	// Intra-project: import starts with this project's module path.
	if strings.HasPrefix(importPath, r.cfg.ModulePath+"/") {
		subpath := strings.TrimPrefix(importPath, r.cfg.ModulePath+"/")
		return r.readFromFS(r.cfg.RootFS, subpath)
	}
	if importPath == r.cfg.ModulePath {
		return r.readFromFS(r.cfg.RootFS, "")
	}

	// External dep: find matching dependency.
	dep := r.findDep(importPath)
	if dep == nil {
		return nil, &ResolveError{Path: importPath, Err: ErrNoDependency}
	}

	subpath := ""
	if len(importPath) > len(dep.Path) {
		subpath = importPath[len(dep.Path)+1:]
	}

	if dep.IsLocal() {
		// Local dep: read from RootFS at the replace path + subpath.
		base := dep.LocalPath
		if subpath != "" {
			base = base + "/" + subpath
		}
		return r.readFromFS(r.cfg.RootFS, base)
	}

	// Remote dep: read from CacheFS at path@version/subpath.
	if r.cfg.CacheFS == nil {
		return nil, &ResolveError{Path: importPath, Err: ErrNoCache}
	}
	cacheDir := dep.Path + "@" + dep.Version
	if subpath != "" {
		cacheDir = cacheDir + "/" + subpath
	}
	return r.readFromFS(r.cfg.CacheFS, cacheDir)
}

// findDep finds the dependency whose path is a prefix of importPath.
// Returns the longest match (most specific).
func (r *Resolver) findDep(importPath string) *Dependency {
	var best *Dependency
	for i := range r.cfg.Deps {
		d := &r.cfg.Deps[i]
		if importPath == d.Path || strings.HasPrefix(importPath, d.Path+"/") {
			if best == nil || len(d.Path) > len(best.Path) {
				best = d
			}
		}
	}
	return best
}

// readFromFS reads module source files. Directory takes precedence
// over single file (matches "directory = module" model).
func (r *Resolver) readFromFS(fsys fs.FS, path string) ([]sourceFile, error) {
	if fsys == nil {
		return nil, &ResolveError{Path: path, Err: ErrNoFS}
	}

	dirPath := path
	if path == "" {
		dirPath = "."
	}
	if files, err := r.readDir(fsys, dirPath); err == nil {
		return files, nil
	}

	if data, err := fs.ReadFile(fsys, path+".scampi"); err == nil {
		return []sourceFile{{name: path + ".scampi", data: data}}, nil
	}

	return nil, &ResolveError{Path: path, Err: ErrModuleNotFound}
}

func (r *Resolver) readDir(fsys fs.FS, dirPath string) ([]sourceFile, error) {
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return nil, err
	}
	var files []sourceFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".scampi") {
			continue
		}
		filePath := dirPath + "/" + e.Name()
		if dirPath == "." {
			filePath = e.Name()
		}
		data, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			continue
		}
		files = append(files, sourceFile{name: filePath, data: data})
	}
	if len(files) == 0 {
		return nil, &ResolveError{Path: dirPath, Err: ErrNoFiles}
	}
	return files, nil
}

func importLeaf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
