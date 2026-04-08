// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"context"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/std"
)

// LoadConfig reads a .scampi file, runs the full lang pipeline
// (lex → parse → check → eval → link), and returns a spec.Config
// ready for the engine.
func LoadConfig(
	ctx context.Context,
	cfgPath string,
	src source.Source,
	reg Registry,
) (spec.Config, error) {
	data, err := src.ReadFile(ctx, cfgPath)
	if err != nil {
		return spec.Config{}, errs.WrapErrf(err, "read %s", cfgPath)
	}

	// Lex + parse.
	l := lex.New(cfgPath, data)
	p := parse.New(l)
	f := p.Parse()
	if errs := l.Errors(); len(errs) > 0 {
		return spec.Config{}, errs[0]
	}
	if errs := p.Errors(); len(errs) > 0 {
		return spec.Config{}, errs[0]
	}

	// Type check.
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		return spec.Config{}, errs.WrapErrf(err, "bootstrap std")
	}
	c := check.New(modules)
	c.Check(f)
	if errs := c.Errors(); len(errs) > 0 {
		return spec.Config{}, errs[0]
	}

	// Evaluate.
	result, errs := eval.Eval(f, data, eval.WithStubs(std.FS))
	if len(errs) > 0 {
		return spec.Config{}, errs[0]
	}

	// Link.
	return Link(result, reg, cfgPath)
}
