// SPDX-License-Identifier: GPL-3.0-only

package testkit

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

var assertionAttrs = []string{
	"file",
	"dir",
	"service",
	"package",
	"symlink",
	"container",
	"command_ran",
	"request",
}

// AssertionBuilder is the Starlark value returned by test.assert.that(t).
// Step packages call its attribute methods to register domain-specific assertions.
// Uses target.Target (interface) so assertions work against any target type.
type AssertionBuilder struct {
	tgt       target.Target
	collector *Collector
}

// Target returns the underlying target for use by assertion implementations.
func (b *AssertionBuilder) Target() target.Target { return b.tgt }

// NewAssertionBuilder returns an AssertionBuilder wrapping tgt and collector.
func NewAssertionBuilder(tgt target.Target, collector *Collector) *AssertionBuilder {
	return &AssertionBuilder{tgt: tgt, collector: collector}
}

func (b *AssertionBuilder) String() string        { return "assert_that" }
func (b *AssertionBuilder) Type() string          { return "assert_that" }
func (b *AssertionBuilder) Freeze()               {}
func (b *AssertionBuilder) Truth() starlark.Bool  { return starlark.True }
func (b *AssertionBuilder) Hash() (uint32, error) { return 0, nil }

func (b *AssertionBuilder) AttrNames() []string { return assertionAttrs }

func (b *AssertionBuilder) Attr(name string) (starlark.Value, error) {
	switch name {
	case "file":
		return starlark.NewBuiltin("assert_that.file", b.builtinFile), nil
	case "dir":
		return starlark.NewBuiltin("assert_that.dir", b.builtinDir), nil
	case "service":
		return starlark.NewBuiltin("assert_that.service", b.builtinService), nil
	case "package":
		return starlark.NewBuiltin("assert_that.package", b.builtinPackage), nil
	case "symlink":
		return starlark.NewBuiltin("assert_that.symlink", b.builtinSymlink), nil
	case "container":
		return starlark.NewBuiltin("assert_that.container", b.builtinContainer), nil
	case "command_ran":
		return starlark.NewBuiltin("assert_that.command_ran", b.builtinCommandRan), nil
	case "request":
		return starlark.NewBuiltin("assert_that.request", b.builtinRequest), nil
	}
	return nil, nil
}

// RegisterAssertion registers an assertion with the collector.
// Step packages call this from assertion value methods once they have resolved
// their check closure against the MemTarget.
func (b *AssertionBuilder) RegisterAssertion(desc string, source spec.SourceSpan, check func() error) {
	b.collector.Add(Assertion{Description: desc, Source: source, Check: check})
}

func (b *AssertionBuilder) builtinFile(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs("file", args, kwargs, 1, &path); err != nil {
		return nil, err
	}
	return &FileAssertion{tgt: b.tgt, path: path, collector: b.collector}, nil
}

func (b *AssertionBuilder) builtinDir(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs("dir", args, kwargs, 1, &path); err != nil {
		return nil, err
	}
	return &DirAssertion{tgt: b.tgt, path: path, collector: b.collector}, nil
}

func (b *AssertionBuilder) builtinService(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs("service", args, kwargs, 1, &name); err != nil {
		return nil, err
	}
	return &ServiceAssertion{tgt: b.tgt, name: name, collector: b.collector}, nil
}

func (b *AssertionBuilder) builtinPackage(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs("package", args, kwargs, 1, &name); err != nil {
		return nil, err
	}
	return &PackageAssertion{tgt: b.tgt, name: name, collector: b.collector}, nil
}

func (b *AssertionBuilder) builtinSymlink(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs("symlink", args, kwargs, 1, &path); err != nil {
		return nil, err
	}
	return &SymlinkAssertion{tgt: b.tgt, path: path, collector: b.collector}, nil
}

func (b *AssertionBuilder) builtinContainer(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackPositionalArgs("container", args, kwargs, 1, &name); err != nil {
		return nil, err
	}
	return &ContainerAssertion{tgt: b.tgt, name: name, collector: b.collector}, nil
}

func (b *AssertionBuilder) builtinRequest(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var method, path string
	if err := starlark.UnpackPositionalArgs("request", args, kwargs, 2, &method, &path); err != nil {
		return nil, err
	}
	mock, ok := b.tgt.(*MockREST)
	if !ok {
		// bare-error: assertion setup error, not engine-reachable
		return nil, errs.Errorf("request assertions require a rest_mock target")
	}
	return &RequestAssertion{mock: mock, method: method, path: path, collector: b.collector}, nil
}

func (b *AssertionBuilder) builtinCommandRan(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var substring string
	if err := starlark.UnpackPositionalArgs("command_ran", args, kwargs, 1, &substring); err != nil {
		return nil, err
	}
	tgt := b.tgt
	// bare-error: assertion check result consumed by test runner
	b.collector.Add(Assertion{
		Description: fmt.Sprintf("command containing %q was executed", substring),
		Check: func() error {
			mt, ok := tgt.(*target.MemTarget)
			if !ok {
				return nil
			}
			for _, cmd := range mt.CommandStrings() {
				if strings.Contains(cmd, substring) {
					return nil
				}
			}
			// bare-error: assertion result
			return errs.Errorf("no command containing %q was executed", substring)
		},
	})
	return starlark.None, nil
}
