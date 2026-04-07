// SPDX-License-Identifier: GPL-3.0-only

package check

import (
	"io/fs"
	"strings"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
)

// BootstrapModules parses and type-checks module stubs from the given
// filesystem. Returns a map of module names (from each file's module
// declaration) to their checked scopes.
//
// The root-level .scampi file is checked first (no imports). Submodule
// files in subdirectories are checked with the root module available
// as an import.
func BootstrapModules(fsys fs.FS) (map[string]*Scope, error) {
	// Phase 1: find and check the root module (top-level .scampi file).
	rootFile, rootName, err := parseRootModule(fsys)
	if err != nil {
		return nil, err
	}
	rootChecker := New(nil)
	rootChecker.Check(rootFile)
	if errs := rootChecker.Errors(); len(errs) > 0 {
		return nil, errs[0]
	}
	rootScope := rootChecker.FileScope()

	modules := map[string]*Scope{
		rootName: rootScope,
	}

	// Phase 2: parse and check each submodule with root available.
	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".scampi") {
			return nil
		}
		// Skip root-level files (already processed).
		if !strings.Contains(path, "/") {
			return nil
		}
		f, parseErr := parseStub(fsys, path)
		if parseErr != nil {
			return parseErr
		}
		c := New(map[string]*Scope{rootName: rootScope})
		c.Check(f)
		if errs := c.Errors(); len(errs) > 0 {
			return errs[0]
		}

		modName := f.Module.Name.Name
		modules[modName] = c.FileScope()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return modules, nil
}

// parseRootModule finds and parses the top-level .scampi file in the
// FS root. Returns the parsed file and its module name.
func parseRootModule(fsys fs.FS) (*ast.File, string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, "", err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".scampi") {
			continue
		}
		f, parseErr := parseStub(fsys, e.Name())
		if parseErr != nil {
			return nil, "", parseErr
		}
		name := "main"
		if f.Module != nil {
			name = f.Module.Name.Name
		}
		return f, name, nil
	}
	return nil, "", fs.ErrNotExist
}

func parseStub(fsys fs.FS, path string) (*ast.File, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	l := lex.New(path, data)
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		return nil, errs[0]
	}
	if errs := p.Errors(); len(errs) > 0 {
		return nil, errs[0]
	}
	return f, nil
}
