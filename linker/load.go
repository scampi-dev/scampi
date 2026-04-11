// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"context"
	"fmt"
	"path/filepath"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/std"
)

// Analysis is the result of Analyze: the parsed AST, the eval result,
// and the original file bytes. Callers needing only diagnostics can
// discard everything and use the error return; callers needing the
// pipeline outputs (linker, LSP doc indexing) consume the fields.
type Analysis struct {
	File   *ast.File
	Result *eval.Result
	Source []byte
}

// Analyze runs the full lang pipeline (lex → parse → check → eval →
// attribute static checks) without performing the final Link step.
// Returns the analysis result and an error wrapping the first
// failing phase's diagnostics. Suitable for both the linker
// (LoadConfig wraps Analyze + Link) and the LSP (which surfaces
// diagnostics inline as the user types).
func Analyze(ctx context.Context, cfgPath string, src source.Source) (*Analysis, error) {
	data, err := src.ReadFile(ctx, cfgPath)
	if err != nil {
		return nil, errs.WrapErrf(err, "read %s", cfgPath)
	}

	// Lex + parse.
	l := lex.New(cfgPath, data)
	p := parse.New(l)
	f := p.Parse()
	if lexErrs := l.Errors(); len(lexErrs) > 0 {
		return nil, wrapLangErrors(lexErrs, cfgPath, data)
	}
	if parseErrs := p.Errors(); len(parseErrs) > 0 {
		return nil, wrapLangErrors(parseErrs, cfgPath, data)
	}

	// Type check.
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		return nil, wrapLangError(err, cfgPath, data)
	}
	c := check.New(modules)
	c.Check(f)
	if checkErrs := c.Errors(); len(checkErrs) > 0 {
		return nil, wrapLangErrors(checkErrs, cfgPath, data)
	}

	// Evaluate with secret backend wiring. The wirer captures the
	// configured backend so the post-eval attribute static check pass
	// can validate literal secret keys against it.
	onEmit, secretsProvider := newSecretWirer(ctx, cfgPath, src)
	result, evalErrs := eval.Eval(f, data, eval.WithStubs(std.FS), eval.WithOnEmit(onEmit))
	if len(evalErrs) > 0 {
		return nil, wrapLangErrors(evalErrs, cfgPath, data)
	}

	// Run attribute static checks. This walks the user file's AST
	// looking for call sites of functions whose parameters carry
	// `@`-attributes, dispatches each to its registered behaviour,
	// and validates literal arguments before plan/apply runs.
	if attrErr := runAttributeStaticChecks(
		f,
		data,
		cfgPath,
		c.FileScope(),
		modules,
		DefaultAttributes(),
		secretsProvider(),
	); attrErr != nil {
		return nil, attrErr
	}

	return &Analysis{
		File:   f,
		Result: result,
		Source: data,
	}, nil
}

// LoadModule parses and type-checks a single stub or user-module
// file against the provided module map. Returns the parsed AST and
// the resulting file scope so callers can extract module metadata
// (catalog entries, goto-def locations, completion sources).
//
// Unlike Analyze, LoadModule does not run the evaluator or attribute
// static checks — stub modules declare types and signatures but have
// no runtime body to evaluate. The LSP uses this to load user-module
// dependencies into its catalog without dragging in eval/link
// machinery.
//
// On lex/parse/check errors the function returns the partial parse
// result alongside a wrapped diagnostic error so callers can choose
// to log-and-skip (LSP) or fail-hard (test harness).
func LoadModule(
	modules map[string]*check.Scope,
	path string,
	data []byte,
) (*ast.File, *check.Scope, error) {
	l := lex.New(path, data)
	p := parse.New(l)
	f := p.Parse()
	if lexErrs := l.Errors(); len(lexErrs) > 0 {
		return f, nil, wrapLangErrors(lexErrs, path, data)
	}
	if parseErrs := p.Errors(); len(parseErrs) > 0 {
		return f, nil, wrapLangErrors(parseErrs, path, data)
	}
	c := check.New(modules)
	c.Check(f)
	scope := c.FileScope()
	if checkErrs := c.Errors(); len(checkErrs) > 0 {
		return f, scope, wrapLangErrors(checkErrs, path, data)
	}
	return f, scope, nil
}

// LoadConfig reads a .scampi file, runs the full lang pipeline
// (lex → parse → check → eval → link), and returns a spec.Config
// ready for the engine.
func LoadConfig(
	ctx context.Context,
	cfgPath string,
	src source.Source,
	reg Registry,
) (spec.Config, error) {
	a, err := Analyze(ctx, cfgPath, src)
	if err != nil {
		return spec.Config{}, err
	}
	return Link(a.Result, reg, cfgPath, WithSourceResolver(ctx, cfgPath, src))
}

// SecretBackendProvider exposes the secret backend captured by
// newSecretWirer after the evaluator has processed a SecretsConfig
// value. Returns nil if no backend has been configured (the user did
// not call std.secrets in their config).
type SecretBackendProvider func() secret.Backend

// newSecretWirer returns an onEmit callback that detects SecretsConfig
// values and wires the secret backend into the evaluator, plus a
// provider that exposes the captured backend after eval has run. The
// post-eval attribute static-check pass uses the provider to validate
// literal secret keys against the configured backend.
func newSecretWirer(
	ctx context.Context,
	cfgPath string,
	src source.Source,
) (eval.EmitCallback, SecretBackendProvider) {
	configured := false
	var captured secret.Backend
	provider := func() secret.Backend { return captured }
	onEmit := func(v eval.Value, ev *eval.Evaluator) {
		sv, ok := v.(*eval.StructVal)
		if !ok || sv.RetType != "SecretsConfig" {
			return
		}
		if configured {
			ev.AddError("secrets() called more than once")
			return
		}
		configured = true
		backend := ""
		if b, ok := sv.Fields["backend"].(*eval.StringVal); ok {
			backend = b.V
		}
		path := ""
		if p, ok := sv.Fields["path"].(*eval.StringVal); ok {
			path = p.V
		}
		if path == "" {
			return
		}
		// Resolve path relative to config file.
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(cfgPath), path)
		}
		data, readErr := src.ReadFile(ctx, path)
		if readErr != nil {
			ev.AddError(fmt.Sprintf("reading secrets file %q: %s", path, readErr))
			return
		}
		var b secret.Backend
		switch backend {
		case "file":
			fb, err := secret.NewFileBackend(data)
			if err == nil {
				b = fb
			}
		case "age":
			readFile := func(p string) ([]byte, error) {
				return src.ReadFile(ctx, p)
			}
			identities, idErr := secret.ResolveIdentities(
				src.LookupEnv,
				readFile,
			)
			if idErr != nil {
				ev.AddError(fmt.Sprintf("secrets backend \"age\": cannot resolve identity keys: %s", idErr))
				return
			}
			ab, abErr := secret.NewAgeBackend(data, identities)
			if abErr != nil {
				ev.AddError(fmt.Sprintf("secrets backend \"age\": cannot decrypt %q: %s", path, abErr))
				return
			}
			b = ab
		}
		if b != nil {
			captured = b
			ev.SetSecretLookup(func(key string) (string, error) {
				v, found, err := b.Lookup(key)
				if err != nil {
					return "", err
				}
				if !found {
					// bare-error: eval callback, no source span available
					return "", errs.New("secret " + key + " not found")
				}
				return v, nil
			})
		}
	}
	return onEmit, provider
}
