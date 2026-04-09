// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"context"
	"fmt"
	"path/filepath"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/eval"
	"scampi.dev/scampi/lang/lex"
	"scampi.dev/scampi/lang/parse"
	"scampi.dev/scampi/secret"
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
	if lexErrs := l.Errors(); len(lexErrs) > 0 {
		return spec.Config{}, wrapLangErrors(lexErrs, cfgPath, data)
	}
	if parseErrs := p.Errors(); len(parseErrs) > 0 {
		return spec.Config{}, wrapLangErrors(parseErrs, cfgPath, data)
	}

	// Type check.
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		return spec.Config{}, wrapLangError(err, cfgPath, data)
	}
	c := check.New(modules)
	c.Check(f)
	if checkErrs := c.Errors(); len(checkErrs) > 0 {
		return spec.Config{}, wrapLangErrors(checkErrs, cfgPath, data)
	}

	// Evaluate with secret backend wiring.
	onEmit := makeSecretWirer(ctx, cfgPath, src)
	result, evalErrs := eval.Eval(f, data, eval.WithStubs(std.FS), eval.WithOnEmit(onEmit))
	if len(evalErrs) > 0 {
		return spec.Config{}, wrapLangErrors(evalErrs, cfgPath, data)
	}

	// Link.
	return Link(result, reg, cfgPath, WithSourceResolver(ctx, cfgPath, src))
}

// makeSecretWirer returns an onEmit callback that detects SecretsConfig
// values and wires the secret backend into the evaluator.
func makeSecretWirer(ctx context.Context, cfgPath string, src source.Source) func(eval.Value, *eval.Evaluator) {
	configured := false
	return func(v eval.Value, ev *eval.Evaluator) {
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
}
