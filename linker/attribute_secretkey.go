// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/spec"
)

// SecretKeyAttribute is the linker behaviour for `@secretkey`. It
// validates that string literal arguments to a `@secretkey`-annotated
// parameter exist in the resolver's backend at link time, surfacing
// missing keys as typed diagnostics with source spans pointing at
// the offending literal.
//
// The backend is extracted from the ResolverBackend field on the
// StaticCheckContext, which is populated from the let-bound
// SecretResolverVal receiver of the UFCS get() call.
//
// Computed (non-literal) arguments are skipped at link time; they
// fall through to the runtime check in lang/eval which already
// validates the resolved value with a source span.
type SecretKeyAttribute struct{}

func (SecretKeyAttribute) StaticCheck(ctx StaticCheckContext) {
	backend := ctx.ResolverBackend
	if backend == nil {
		return
	}
	literal := stringLiteralValue(ctx.ParamArg)
	if literal == "" {
		return
	}
	_, found, err := backend.Lookup(literal)
	if err != nil {
		ctx.Linker.Raise(&secretKeyLookupError{
			Key: literal,
			Err: err,
			Src: &ctx.UseSpan,
		})
		return
	}
	if !found {
		ctx.Linker.Raise(&secretKeyNotFoundError{
			Key: literal,
			Src: &ctx.UseSpan,
		})
	}
}

// stringLiteralValue extracts the literal string value from an
// expression if it is a single-segment string literal with no
// interpolation. Returns "" for any other shape (computed strings,
// concatenations, function calls, etc.) — those defer to the runtime
// check in lang/eval.
func stringLiteralValue(e ast.Expr) string {
	sl, ok := e.(*ast.StringLit)
	if !ok || len(sl.Parts) != 1 {
		return ""
	}
	text, ok := sl.Parts[0].(*ast.StringText)
	if !ok {
		return ""
	}
	return text.Raw
}

// Diagnostic data carriers for templates. The TestAllTemplatesRender
// test requires Data to be a struct type so zero values still resolve
// every template field.

type secretKeyNotFoundData struct {
	Key string
}

type secretKeyLookupData struct {
	Key string
	Err string
}

// secretKeyNotFoundError is the diagnostic emitted when a literal
// secret key is not present in the configured backend.
type secretKeyNotFoundError struct {
	Key string
	Src *spec.SourceSpan
}

func (e *secretKeyNotFoundError) Error() string {
	return "secret key not found: " + e.Key
}

func (e *secretKeyNotFoundError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeSecretKeyNotFound,
			Text:   `secret key {{printf "%q" .Key}} not found in backend`,
			Hint:   "check secrets.from_age/from_file path or add the key to your secrets file",
			Source: e.Src,
			Data:   secretKeyNotFoundData{Key: e.Key},
		},
	}
}

// secretKeyLookupError is the diagnostic emitted when the secrets
// backend itself errors during a lookup (e.g. decryption failure).
type secretKeyLookupError struct {
	Key string
	Err error
	Src *spec.SourceSpan
}

func (e *secretKeyLookupError) Error() string {
	return "secret key lookup failed for " + e.Key + ": " + e.Err.Error()
}

func (e *secretKeyLookupError) Diagnostic() event.Event {
	return event.Error{
		Impact: event.ImpactAbort,
		Template: event.Template{
			ID:     CodeSecretKeyLookupFailed,
			Text:   `secret key {{printf "%q" .Key}} lookup failed: {{.Err}}`,
			Source: e.Src,
			Data: secretKeyLookupData{
				Key: e.Key,
				Err: e.Err.Error(),
			},
		},
	}
}
