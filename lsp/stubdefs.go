// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/lang/token"
	"scampi.dev/scampi/std"
)

// StubDefs extracts stdlib stubs to a versioned cache directory and
// indexes declaration locations so goto-definition works for stdlib
// symbols.
//
// Two indexes:
//   - locs: qualified decl/func/type name → its definition span
//     (e.g. `posix.copy` → posix.scampi:65:6)
//   - params: `<qualifiedName>/<paramName>` → param span
//     (e.g. `posix.copy/perm` → posix.scampi:67:3) — used for
//     goto-def from a struct-literal field reference.
type StubDefs struct {
	dir    string                  // cache dir holding extracted stubs
	locs   map[string]stubLocation // qualified name → location
	params map[string]stubLocation // "qname/paramName" → location
}

type stubLocation struct {
	path string     // absolute path to the extracted stub file
	src  []byte     // file content (for span resolution)
	span token.Span // span of the declaration name
}

// NewStubDefs extracts stubs from std.FS to a versioned cache directory
// and indexes all declaration locations.
func NewStubDefs() *StubDefs {
	dir := filepath.Join(cacheBase(), "scampls", "stubs", Version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &StubDefs{
			locs:   map[string]stubLocation{},
			params: map[string]stubLocation{},
		}
	}

	sd := &StubDefs{
		dir:    dir,
		locs:   make(map[string]stubLocation),
		params: make(map[string]stubLocation),
	}
	sd.extract()
	return sd
}

// Lookup returns a goto-definition location for the given qualified name.
func (sd *StubDefs) Lookup(name string) (protocol.Location, bool) {
	sl, ok := sd.locs[name]
	if !ok {
		return protocol.Location{}, false
	}
	return protocol.Location{
		URI:   uri.File(sl.path),
		Range: tokenSpanToRange(sl.src, sl.span),
	}, true
}

// LookupParam returns a goto-definition location for a parameter of
// a stub func/decl/type. qname is the qualified name (e.g.
// "posix.copy"), paramName is the field/param identifier
// (e.g. "perm"). Used by goto-def on struct-literal field names.
func (sd *StubDefs) LookupParam(qname, paramName string) (protocol.Location, bool) {
	sl, ok := sd.params[qname+"/"+paramName]
	if !ok {
		return protocol.Location{}, false
	}
	return protocol.Location{
		URI:   uri.File(sl.path),
		Range: tokenSpanToRange(sl.src, sl.span),
	}, true
}

// cacheBase returns the base cache directory, respecting XDG_CACHE_HOME
// before falling back to os.UserCacheDir.
func cacheBase() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return xdg
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return dir
}

// indexParams registers each field/param under "<qname>/<paramName>"
// so goto-def from a struct-literal field jumps to the param declaration.
func (sd *StubDefs) indexParams(qname string, fields []*ast.Field, outPath string, data []byte) {
	for _, f := range fields {
		if f == nil || f.Name == nil {
			continue
		}
		key := qname + "/" + f.Name.Name
		sd.params[key] = stubLocation{
			path: outPath,
			src:  data,
			span: f.Name.SrcSpan,
		}
	}
}

func (sd *StubDefs) extract() {
	_ = fs.WalkDir(std.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".scampi") {
			return nil
		}
		data, err := fs.ReadFile(std.FS, path)
		if err != nil {
			return nil
		}

		outPath := filepath.Join(sd.dir, path)
		_ = os.MkdirAll(filepath.Dir(outPath), 0o755)
		// Remove any prior version first. Earlier builds wrote with
		// mode 0o444 (read-only), which makes os.WriteFile silently
		// fail on rewrite — leaving the cache permanently stale.
		// Removing first guarantees a clean rewrite even when the
		// existing file was created by an old binary.
		_ = os.Remove(outPath)
		_ = os.WriteFile(outPath, data, 0o644)

		l := lex.New(path, data)
		p := parse.New(l)
		f := p.Parse()
		if f == nil {
			return nil
		}

		modName := "main"
		if f.Module != nil {
			modName = f.Module.Name.Name
		}

		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				qn := modName + "." + d.Name.Name
				sd.locs[qn] = stubLocation{path: outPath, src: data, span: d.Name.SrcSpan}
				sd.indexParams(qn, d.Params, outPath, data)
			case *ast.DeclDecl:
				name := d.Name.Parts[0].Name
				qn := modName + "." + name
				sd.locs[qn] = stubLocation{path: outPath, src: data, span: d.Name.SrcSpan}
				sd.indexParams(qn, d.Params, outPath, data)
			case *ast.TypeDecl:
				qn := modName + "." + d.Name.Name
				sd.locs[qn] = stubLocation{path: outPath, src: data, span: d.Name.SrcSpan}
				sd.indexParams(qn, d.Fields, outPath, data)
			case *ast.EnumDecl:
				qn := modName + "." + d.Name.Name
				sd.locs[qn] = stubLocation{path: outPath, src: data, span: d.Name.SrcSpan}
			case *ast.AttrTypeDecl:
				// Attribute types are looked up by their `@`-prefixed
				// form. We register both the bare reference (the user
				// typed `@secretkey`) and the qualified one
				// (`@std.secretkey`) so goto-def works either way.
				bare := "@" + d.Name.Name
				qualified := "@" + modName + "." + d.Name.Name
				loc := stubLocation{path: outPath, src: data, span: d.Name.SrcSpan}
				sd.locs[bare] = loc
				sd.locs[qualified] = loc
				sd.indexParams(qualified, d.Fields, outPath, data)
			}
		}

		return nil
	})
}
