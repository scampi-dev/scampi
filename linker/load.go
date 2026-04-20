// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"context"
	"encoding/json"
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

	// Type check. Bootstrap std modules first, then load any user
	// modules declared in scampi.mod so `import "mymodule"` works.
	modules, err := check.BootstrapModules(std.FS)
	if err != nil {
		return nil, wrapLangError(err, cfgPath, data)
	}
	userMods := LoadUserModules(cfgPath, modules)

	// Multi-file module detection: if the file declares a non-main
	// module and there are sibling .scampi files with the same
	// module name, load them all into a shared scope (Go package
	// model). This makes `scampi check module_file.scampi` work
	// for files that reference siblings.
	modName := "main"
	if f.Module != nil {
		modName = f.Module.Name.Name
	}
	var siblingMods []eval.UserModule
	var brokenSiblings []brokenSibling
	c := check.New(modules)
	if modName != "main" {
		siblings, broken := loadSiblingDecls(cfgPath, modName, modules)
		brokenSiblings = append(brokenSiblings, broken...)
		if siblings != nil {
			c.WithScope(siblings)
		}
		var broken2 []brokenSibling
		siblingMods, broken2 = loadSiblingUserModules(cfgPath, modName, modules)
		brokenSiblings = append(brokenSiblings, broken2...)
	}
	c.Check(f)
	if checkErrs := c.Errors(); len(checkErrs) > 0 {
		wrapped := wrapLangErrors(checkErrs, cfgPath, data)
		return nil, prependBrokenSiblings(wrapped, brokenSiblings)
	}

	// Evaluate with secret backend builtins registered so
	// secrets.from_age / secrets.from_file / secrets.get work.
	readFile := func(path string) ([]byte, error) {
		return src.ReadFile(ctx, path)
	}
	configDir := filepath.Dir(cfgPath)
	result, evalErrs := eval.Eval(
		f,
		data,
		eval.WithStubs(std.FS),
		eval.WithUserModules(userMods),
		eval.WithSiblingModules(siblingMods),
		eval.WithEnv(src.LookupEnv),
		eval.WithBuiltinFunc("secrets.from_age", secretFromAge(configDir, src.LookupEnv, readFile)),
		eval.WithBuiltinFunc("secrets.from_file", secretFromFile(configDir, readFile)),
		eval.WithBuiltinFunc("secrets.get", secretGetBuiltin()),
	)
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
		result,
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

// secretFromAge returns a BuiltinFunc for secrets.from_age(path).
func secretFromAge(
	configDir string,
	envLookup func(string) (string, bool),
	readFile func(string) ([]byte, error),
) eval.BuiltinFunc {
	return func(positional []eval.Value, kwargs map[string]eval.Value) (eval.Value, string) {
		path := stringArg(positional, kwargs, "path")
		if path == "" {
			return nil, "secrets.from_age requires a path argument"
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}
		data, err := readFile(path)
		if err != nil {
			return nil, "reading secrets file: " + err.Error()
		}
		var raw map[string]string
		if jsonErr := json.Unmarshal(data, &raw); jsonErr != nil {
			return nil, "parsing secrets file: " + jsonErr.Error()
		}
		lookup := envLookup
		if lookup == nil {
			lookup = func(string) (string, bool) { return "", false }
		}
		identities, idErr := secret.ResolveIdentities(lookup, readFile)
		if idErr != nil {
			keys := make(map[string]string, len(raw))
			for k := range raw {
				keys[k] = "<secret>"
			}
			return &eval.OpaqueVal{
				TypeName: "SecretResolver",
				Inner: &secret.PlaceholderBackend{
					KeyMap: keys,
					Cause:  idErr,
				},
			}, ""
		}
		ab, abErr := secret.NewAgeBackend(data, identities)
		if abErr != nil {
			return nil, "age backend: " + abErr.Error()
		}
		return &eval.OpaqueVal{TypeName: "SecretResolver", Inner: ab}, ""
	}
}

// secretFromFile returns a BuiltinFunc for secrets.from_file(path).
func secretFromFile(
	configDir string,
	readFile func(string) ([]byte, error),
) eval.BuiltinFunc {
	return func(positional []eval.Value, kwargs map[string]eval.Value) (eval.Value, string) {
		path := stringArg(positional, kwargs, "path")
		if path == "" {
			return nil, "secrets.from_file requires a path argument"
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}
		data, err := readFile(path)
		if err != nil {
			return nil, "reading secrets file: " + err.Error()
		}
		fb, fbErr := secret.NewFileBackend(data)
		if fbErr != nil {
			return nil, "file backend: " + fbErr.Error()
		}
		return &eval.OpaqueVal{TypeName: "SecretResolver", Inner: fb}, ""
	}
}

// secretGetBuiltin returns a BuiltinFunc for secrets.get(resolver, key).
func secretGetBuiltin() eval.BuiltinFunc {
	return func(positional []eval.Value, kwargs map[string]eval.Value) (eval.Value, string) {
		if len(positional) == 0 {
			return nil, "secrets.get requires a resolver and key"
		}
		opaque, ok := positional[0].(*eval.OpaqueVal)
		if !ok {
			return nil, "first argument to secrets.get must be a SecretResolver"
		}
		b, ok := opaque.Inner.(secret.Backend)
		if !ok {
			return nil, "invalid secret backend"
		}
		key := stringArg(positional[1:], kwargs, "key")
		v, found, err := b.Lookup(key)
		if err != nil {
			return nil, "secret lookup failed: " + err.Error()
		}
		if !found {
			return nil, "secret key " + key + " not found"
		}
		return &eval.StringVal{V: v}, ""
	}
}

// stringArg extracts a string from the first positional arg or the
// named kwarg.
func stringArg(positional []eval.Value, kwargs map[string]eval.Value, name string) string {
	if len(positional) > 0 {
		if s, ok := positional[0].(*eval.StringVal); ok {
			return s.V
		}
	}
	if v, ok := kwargs[name]; ok {
		if s, ok := v.(*eval.StringVal); ok {
			return s.V
		}
	}
	return ""
}
