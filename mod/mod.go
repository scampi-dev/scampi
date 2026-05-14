// SPDX-License-Identifier: GPL-3.0-only

// Package mod parses and represents scampi.mod module manifests.
package mod

import (
	"strings"
	"unicode"

	"scampi.dev/scampi/spec"
)

// Module is the parsed representation of a scampi.mod file.
type Module struct {
	Module     string
	ModuleLine int
	Require    []Dependency
	Filename   string
}

// Dependency is a single entry in the require block.
type Dependency struct {
	Path     string
	Version  string
	Line     int
	Indirect bool
}

// IsLocal reports whether this dependency points to a local directory
// rather than a remote git repository.
func (d Dependency) IsLocal() bool {
	return strings.HasPrefix(d.Version, ".") ||
		strings.HasPrefix(d.Version, "/")
}

// span builds a SourceSpan pointing to a specific line in this module file.
func (m *Module) span(line int) spec.SourceSpan {
	return spec.SourceSpan{
		Filename:  m.Filename,
		StartLine: line,
		EndLine:   line,
	}
}

// DepSpan returns a SourceSpan for a dependency entry.
func (m *Module) DepSpan(dep *Dependency) spec.SourceSpan {
	return m.span(dep.Line)
}

// HasDep reports whether the module's require table contains an entry for path.
func (m *Module) HasDep(path string) bool {
	for _, dep := range m.Require {
		if dep.Path == path || strings.HasPrefix(path, dep.Path+"/") {
			return true
		}
	}
	return false
}

// IsModulePath reports whether path looks like a valid module path:
// must contain at least one dot in the first path segment (host), and have
// at least one further path segment.
func IsModulePath(path string) bool {
	return isModulePath(path)
}

// Validation helpers
// -----------------------------------------------------------------------------

func isModulePath(p string) bool {
	if p == "" {
		return false
	}
	host, rest, ok := strings.Cut(p, "/")
	if !ok {
		return false
	}
	if !strings.Contains(host, ".") {
		return false
	}
	return rest != ""
}

// Parse
// -----------------------------------------------------------------------------

// Parse parses scampi.mod content. filename is used for source spans in errors.
func Parse(filename string, data []byte) (*Module, error) {
	m := &Module{Filename: filename}

	lines := strings.Split(string(data), "\n")

	inRequire := false
	requireOpenLine := 0

	for i, raw := range lines {
		lineNum := i + 1
		line := strings.TrimSpace(raw)

		// Detect // indirect before stripping inline comments.
		indirect := strings.Contains(line, "// indirect")

		// Strip inline comments
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		if line == "" {
			continue
		}

		if inRequire {
			if line == ")" {
				inRequire = false
				continue
			}
			dep, err := parseDependency(m, line, lineNum, indirect)
			if err != nil {
				return nil, err
			}
			if dep != nil {
				m.Require = append(m.Require, *dep)
			}
			continue
		}

		if strings.HasPrefix(line, "module ") {
			if m.Module != "" {
				return nil, ParseError{
					Detail: "duplicate module directive",
					Hint:   "remove the duplicate — only one module directive is allowed",
					Source: m.span(lineNum),
				}
			}
			path := strings.TrimSpace(line[len("module "):])
			if !isModulePath(path) {
				return nil, ParseError{
					Detail: "invalid module path " + quote(path),
					Hint: "module path must be a host/path URL, e.g. " +
						"module codeberg.org/" + nonEmpty(path, "yourname/yourmodule"),
					Source: m.span(lineNum),
				}
			}
			m.Module = path
			m.ModuleLine = lineNum
			continue
		}

		if line == "require (" {
			inRequire = true
			requireOpenLine = lineNum
			continue
		}

		if strings.HasPrefix(line, "require ") {
			// single-line require: require codeberg.org/foo/bar v1.0.0
			rest := strings.TrimSpace(line[len("require "):])
			dep, err := parseDependency(m, rest, lineNum, indirect)
			if err != nil {
				return nil, err
			}
			if dep != nil {
				m.Require = append(m.Require, *dep)
			}
			continue
		}

		return nil, ParseError{
			Detail: "unexpected token " + quote(firstWord(line)),
			Hint:   "scampi.mod supports only module and require directives",
			Source: m.span(lineNum),
		}
	}

	if inRequire {
		return nil, ParseError{
			Detail: "unclosed require block",
			Hint:   "add a closing ) to end the require block",
			Source: m.span(requireOpenLine),
		}
	}

	if m.Module == "" {
		return nil, ParseError{
			Detail: "missing module directive",
			Hint:   "add a module directive as the first line, e.g. module codeberg.org/yourname/yourmodule",
			Source: spec.SourceSpan{Filename: filename},
		}
	}

	return m, nil
}

func parseDependency(m *Module, line string, lineNum int, indirect bool) (*Dependency, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil, nil
	}
	if len(fields) < 2 {
		return nil, ParseError{
			Detail: "malformed require entry " + quote(line),
			Hint:   "require entries must be: " + nonEmpty(fields[0], "codeberg.org/example/module") + " v1.0.0",
			Source: m.span(lineNum),
		}
	}
	path, version := fields[0], fields[1]
	isLocal := strings.HasPrefix(version, ".") || strings.HasPrefix(version, "/")

	if !isLocal && !isModulePath(path) {
		return nil, ParseError{
			Detail: "invalid module path " + quote(path),
			Hint:   "module path must be a host/path URL, e.g. codeberg.org/" + nonEmpty(path, "example/module"),
			Source: m.span(lineNum),
		}
	}
	// Version can be a semver tag (v1.0.0), branch (main), or
	// module-prefixed tag (npm-v0.1.0). No strict validation —
	// git clone will reject truly invalid refs downstream.
	return &Dependency{Path: path, Version: version, Line: lineNum, Indirect: indirect}, nil
}

// String helpers
// -----------------------------------------------------------------------------

func quote(s string) string { return `"` + s + `"` }

func firstWord(s string) string {
	if idx := strings.IndexFunc(s, unicode.IsSpace); idx >= 0 {
		return s[:idx]
	}
	return s
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
