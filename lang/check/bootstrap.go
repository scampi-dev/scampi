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
// Submodule stubs may import other submodules — bootstrap iterates
// to a fixed point so the order in which fs.WalkDir visits files
// doesn't matter. Each pass checks every module whose imports are
// already satisfied; the loop terminates when either every module
// has been checked or a pass made no progress (in which case the
// remaining modules have unresolved imports — e.g. an actual cycle
// or a typo — and we surface their first error).
func BootstrapModules(fsys fs.FS) (map[string]*Scope, error) {
	rootFile, rootName, err := parseRootModule(fsys)
	if err != nil {
		return nil, err
	}
	rootChecker := New(nil)
	rootChecker.Check(rootFile)
	if errs := rootChecker.Errors(); len(errs) > 0 {
		return nil, errs[0]
	}

	modules := map[string]*Scope{
		rootName: rootChecker.FileScope(),
	}

	pending, err := parseSubmodules(fsys)
	if err != nil {
		return nil, err
	}

	// Fixed-point loop. Each pass tries to check every still-pending
	// module against the current `modules` map. If a module checks
	// cleanly, it joins `modules` and is removed from `pending`. If
	// no module makes progress on a pass, the remaining ones are
	// either cyclic or refer to modules that don't exist — bail with
	// the first error we hit.
	for len(pending) > 0 {
		progressed := false
		for path, file := range pending {
			c := New(modules)
			c.Check(file)
			if errs := c.Errors(); len(errs) > 0 {
				continue
			}
			modules[file.Module.Name.Name] = c.FileScope()
			delete(pending, path)
			progressed = true
		}
		if !progressed {
			// Pick any remaining module and surface its real error
			// — at this point its imports definitely won't resolve,
			// so the checker error is the meaningful one.
			for _, file := range pending {
				c := New(modules)
				c.Check(file)
				if errs := c.Errors(); len(errs) > 0 {
					return nil, errs[0]
				}
			}
			break
		}
	}

	return modules, nil
}

// parseSubmodules walks the filesystem and parses every non-root
// .scampi file it finds, returning them keyed by path so the caller
// can drive the type-check loop.
func parseSubmodules(fsys fs.FS) (map[string]*ast.File, error) {
	pending := make(map[string]*ast.File)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".scampi") {
			return nil
		}
		// Skip root-level files (the root module is parsed separately).
		if !strings.Contains(path, "/") {
			return nil
		}
		f, parseErr := parseStub(fsys, path)
		if parseErr != nil {
			return parseErr
		}
		if f.Module == nil {
			return nil
		}
		pending[path] = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pending, nil
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
