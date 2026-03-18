// SPDX-License-Identifier: GPL-3.0-only

// Package star evaluates Starlark configuration files into spec.Config.
package star

import (
	"errors"
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/signal"
	"scampi.dev/scampi/spec"
)

// StarlarkError wraps a Starlark evaluation error with source position.
type StarlarkError struct {
	Err    error
	Source spec.SourceSpan
}

func (e StarlarkError) Error() string { return e.Err.Error() }
func (e StarlarkError) Unwrap() error { return e.Err }

func (e StarlarkError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.EvalError",
		Text:   "{{.Err}}",
		Data:   e,
		Source: &e.Source,
	}
}

func (e StarlarkError) Severity() signal.Severity {
	return signal.Error
}

func (e StarlarkError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

// wrapStarlarkError converts a starlark.EvalError into a diagnostic error.
// If the underlying cause already implements diagnostic.Diagnostic, it is
// returned directly (with Source filled from the backtrace if empty).
// Otherwise the error is wrapped in a generic StarlarkError.
func wrapStarlarkError(err error, c *Collector) error {
	var evalErr *starlark.EvalError
	if ok := isEvalError(err, &evalErr); ok && evalErr != nil {
		bt := evalErr.CallStack
		span := spanFromBacktrace(bt)

		cause := evalErr.Unwrap()

		var diag diagnostic.Diagnostic
		if errors.As(cause, &diag) {
			if ss, ok := diag.(sourceSettable); ok {
				ss.setSource(span)
			}
			if pe, ok := cause.(*PoisonValueError); ok {
				refinePoisonSpan(pe, c, span)
			}
			return cause
		}

		if uf, ok := asUnknownFieldError(c, span, cause); ok {
			return uf
		}

		return StarlarkError{Err: err, Source: span}
	}
	return StarlarkError{Err: err}
}

// asUnknownFieldError attempts to convert an UnpackArgs "unexpected keyword
// argument" error into an UnknownFieldError with precise source position.
func asUnknownFieldError(c *Collector, callSite spec.SourceSpan, cause error) (*UnknownFieldError, bool) {
	if c == nil || cause == nil {
		return nil, false
	}
	field, suggestion := parseUnexpectedKwarg(cause.Error())
	if field == "" {
		return nil, false
	}

	source := callSite
	if f := c.AST(callSite.Filename); f != nil {
		pos := posFromSpan(callSite)
		if call := findCallExpr(f, pos); call != nil {
			if refined, ok := kwargKeySpan(call, field); ok {
				source = refined
			}
		}
	}

	return &UnknownFieldError{
		Field:      field,
		Suggestion: suggestion,
		Source:     source,
	}, true
}

// parseUnexpectedKwarg extracts the field name and optional suggestion from an
// UnpackArgs error like `pkg: unexpected keyword argument "packagess" (did you mean packages?)`.
func parseUnexpectedKwarg(msg string) (field, suggestion string) {
	const marker = "unexpected keyword argument "
	idx := strings.Index(msg, marker)
	if idx < 0 {
		return "", ""
	}
	rest := msg[idx+len(marker):]

	// Extract the field name (quoted by Starlark's String.String())
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		field = strings.Trim(rest[:i], `"`)
		rest = rest[i:]
	} else {
		return strings.Trim(rest, `"`), ""
	}

	// Extract "did you mean X?" suggestion
	const didYouMean = "(did you mean "
	if i := strings.Index(rest, didYouMean); i >= 0 {
		s := rest[i+len(didYouMean):]
		if j := strings.IndexByte(s, '?'); j >= 0 {
			suggestion = s[:j]
		}
	}

	return field, suggestion
}

// spanFromBacktrace walks the call stack from innermost to outermost and
// returns the first frame with a real source file (skipping <builtin> etc).
func spanFromBacktrace(bt starlark.CallStack) spec.SourceSpan {
	for i := len(bt) - 1; i >= 0; i-- {
		pos := bt[i].Pos
		if pos.Filename() != "" && pos.Filename() != "<builtin>" {
			return posToSpan(pos)
		}
	}
	if len(bt) > 0 {
		return posToSpan(bt[len(bt)-1].Pos)
	}
	return spec.SourceSpan{}
}

func isEvalError(err error, target **starlark.EvalError) bool {
	for err != nil {
		if ee, ok := err.(*starlark.EvalError); ok {
			*target = ee
			return true
		}
		if u, ok := err.(interface{ Unwrap() error }); ok {
			err = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}

// sourceSettable allows wrapStarlarkError to fill Source from the EvalError
// backtrace on diagnostic types that left it empty.
type sourceSettable interface {
	setSource(spec.SourceSpan)
}

// UnknownFieldError is raised when a call contains an unrecognized field name.
type UnknownFieldError struct {
	Field      string
	Suggestion string
	Source     spec.SourceSpan
}

func (e UnknownFieldError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("unknown field %q (did you mean %q?)", e.Field, e.Suggestion)
	}
	return fmt.Sprintf("unknown field %q", e.Field)
}

func (e UnknownFieldError) EventTemplate() event.Template {
	tmpl := event.Template{
		ID:     "star.UnknownField",
		Text:   `unknown field "{{.Field}}"`,
		Data:   e,
		Source: &e.Source,
	}
	if e.Suggestion != "" {
		tmpl.Hint = fmt.Sprintf("did you mean %q?", e.Suggestion)
	}
	return tmpl
}

func (e UnknownFieldError) Severity() signal.Severity {
	return signal.Error
}

func (e UnknownFieldError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

// DuplicateTargetError is raised when a target name is registered twice.
type DuplicateTargetError struct {
	Name   string
	Source spec.SourceSpan
}

func (e DuplicateTargetError) Error() string {
	return fmt.Sprintf("duplicate target %q", e.Name)
}

func (e DuplicateTargetError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.DuplicateTarget",
		Text:   `duplicate target "{{.Name}}"`,
		Hint:   "each target name must be unique",
		Data:   e,
		Source: &e.Source,
	}
}

func (e DuplicateTargetError) Severity() signal.Severity {
	return signal.Error
}

func (e DuplicateTargetError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func (e *DuplicateTargetError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// DuplicateDeployError is raised when a deploy block name is registered twice.
type DuplicateDeployError struct {
	Name   string
	Source spec.SourceSpan
}

func (e DuplicateDeployError) Error() string {
	return fmt.Sprintf("duplicate deploy block %q", e.Name)
}

func (e DuplicateDeployError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.DuplicateDeploy",
		Text:   `duplicate deploy block "{{.Name}}"`,
		Hint:   "each deploy block name must be unique",
		Data:   e,
		Source: &e.Source,
	}
}

func (e DuplicateDeployError) Severity() signal.Severity {
	return signal.Error
}

func (e DuplicateDeployError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func (e *DuplicateDeployError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// MissingArgError is raised when a required argument is not provided.
type MissingArgError struct {
	Func   string
	Arg    string
	Source spec.SourceSpan
}

func (e MissingArgError) Error() string {
	return fmt.Sprintf("%s() missing required argument %q", e.Func, e.Arg)
}

func (e MissingArgError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.MissingArg",
		Text:   `{{.Func}}() missing required argument "{{.Arg}}"`,
		Data:   e,
		Source: &e.Source,
	}
}

func (e MissingArgError) Severity() signal.Severity {
	return signal.Error
}

func (e MissingArgError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func (e *MissingArgError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// EnvVarRequiredError is raised when a required env var is unset.
type EnvVarRequiredError struct {
	Key    string
	Source spec.SourceSpan
}

func (e EnvVarRequiredError) Error() string {
	return fmt.Sprintf("required environment variable %q is not set", e.Key)
}

func (e EnvVarRequiredError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.EnvVarRequired",
		Text:   `required environment variable "{{.Key}}" is not set`,
		Hint:   "set the variable or provide a default value",
		Data:   e,
		Source: &e.Source,
	}
}

func (e EnvVarRequiredError) Severity() signal.Severity {
	return signal.Error
}

func (e EnvVarRequiredError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func (e *EnvVarRequiredError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// FileReadError is raised when a configuration file cannot be read.
type FileReadError struct {
	Path   string
	Cause  error
	Source spec.SourceSpan
}

func (e FileReadError) Error() string {
	return fmt.Sprintf("reading %s: %s", e.Path, e.Cause)
}

func (e FileReadError) Unwrap() error { return e.Cause }

func (e FileReadError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.FileRead",
		Text:   "reading {{.Path}}: {{.Cause}}",
		Hint:   "check that the file exists and is readable",
		Data:   e,
		Source: &e.Source,
	}
}

func (e FileReadError) Severity() signal.Severity {
	return signal.Error
}

func (e FileReadError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func (e *FileReadError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// TypeError is raised when a value has the wrong Starlark type.
type TypeError struct {
	Context  string
	Expected string
	Got      string
	Source   spec.SourceSpan
}

func (e TypeError) Error() string {
	if e.Context != "" {
		return fmt.Sprintf("%s: expected %s, got %s", e.Context, e.Expected, e.Got)
	}
	return fmt.Sprintf("expected %s, got %s", e.Expected, e.Got)
}

func (e TypeError) EventTemplate() event.Template {
	if e.Context != "" {
		return event.Template{
			ID:     "star.TypeError",
			Text:   "{{.Context}}: expected {{.Expected}}, got {{.Got}}",
			Hint:   "expected {{.Expected}}, got {{.Got}}",
			Data:   e,
			Source: &e.Source,
		}
	}
	return event.Template{
		ID:     "star.TypeError",
		Text:   "expected {{.Expected}}, got {{.Got}}",
		Hint:   "expected {{.Expected}}, got {{.Got}}",
		Data:   e,
		Source: &e.Source,
	}
}

func (e TypeError) Severity() signal.Severity {
	return signal.Error
}

func (e TypeError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func (e *TypeError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// EnvError is raised for invalid env() builtin usage.
type EnvError struct {
	Detail string
	Source spec.SourceSpan
}

func (e EnvError) Error() string {
	return fmt.Sprintf("env: %s", e.Detail)
}

func (e EnvError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.EnvError",
		Text:   "env: {{.Detail}}",
		Data:   e,
		Source: &e.Source,
	}
}

func (e EnvError) Severity() signal.Severity {
	return signal.Error
}

func (e EnvError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func (e *EnvError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// UnknownKeyError is raised when a dict contains an unrecognized key.
type UnknownKeyError struct {
	Key     string
	Allowed []string
	Source  spec.SourceSpan
}

func (e UnknownKeyError) Error() string {
	return fmt.Sprintf("unknown key %q", e.Key)
}

func (e UnknownKeyError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.UnknownKey",
		Text:   `unknown key "{{.Key}}"`,
		Hint:   `expected one of: {{join ", " .Allowed}}`,
		Data:   e,
		Source: &e.Source,
	}
}

func (e UnknownKeyError) Severity() signal.Severity {
	return signal.Error
}

func (e UnknownKeyError) Impact() diagnostic.Impact {
	return diagnostic.ImpactAbort
}

func (e *UnknownKeyError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// CircularLoadError is raised when load() forms a cycle.
type CircularLoadError struct {
	Path   string
	Source spec.SourceSpan
}

func (e CircularLoadError) Error() string {
	return fmt.Sprintf("circular load: %s", e.Path)
}

func (e CircularLoadError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.CircularLoad",
		Text:   "circular load: {{.Path}}",
		Hint:   `check load() calls in "{{.Path}}" - one imports a file that eventually loads this file again`,
		Data:   e,
		Source: &e.Source,
	}
}

func (CircularLoadError) Severity() signal.Severity { return signal.Error }
func (CircularLoadError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// EmptyNameError is raised when a required name argument is empty.
type EmptyNameError struct {
	Func   string
	Source spec.SourceSpan
}

func (e EmptyNameError) Error() string {
	return fmt.Sprintf("%s: name must not be empty", e.Func)
}

func (e EmptyNameError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.EmptyName",
		Text:   "{{.Func}}: name must not be empty",
		Data:   e,
		Source: &e.Source,
	}
}

func (EmptyNameError) Severity() signal.Severity { return signal.Error }
func (EmptyNameError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// EmptyListError is raised when a required list argument is empty.
type EmptyListError struct {
	Func   string
	Field  string
	Source spec.SourceSpan
}

func (e EmptyListError) Error() string {
	return fmt.Sprintf("%s: %s must not be empty", e.Func, e.Field)
}

func (e EmptyListError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.EmptyList",
		Text:   "{{.Func}}: {{.Field}} must not be empty",
		Data:   e,
		Source: &e.Source,
	}
}

func (EmptyListError) Severity() signal.Severity { return signal.Error }
func (EmptyListError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

// SecretError is raised for invalid secret() builtin usage.
type SecretError struct {
	Detail string
	Source spec.SourceSpan
}

func (e SecretError) Error() string {
	return fmt.Sprintf("secret: %s", e.Detail)
}

func (e SecretError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.SecretError",
		Text:   "secret: {{.Detail}}",
		Data:   e,
		Source: &e.Source,
	}
}

func (SecretError) Severity() signal.Severity { return signal.Error }
func (SecretError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

func (e *SecretError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// SecretNotFoundError is raised when a secret key is not in the backend.
type SecretNotFoundError struct {
	Key    string
	Source spec.SourceSpan
}

func (e SecretNotFoundError) Error() string {
	return fmt.Sprintf("secret %q not found", e.Key)
}

func (e SecretNotFoundError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.SecretNotFound",
		Text:   `secret "{{.Key}}" not found`,
		Hint:   "add the key to your secrets file or check the backend configuration",
		Data:   e,
		Source: &e.Source,
	}
}

func (SecretNotFoundError) Severity() signal.Severity { return signal.Error }
func (SecretNotFoundError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

func (e *SecretNotFoundError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// SecretBackendError is raised when the secret backend returns an error.
type SecretBackendError struct {
	Key    string
	Cause  error
	Source spec.SourceSpan
}

func (e SecretBackendError) Error() string {
	return fmt.Sprintf("secret backend error for %q: %s", e.Key, e.Cause)
}

func (e SecretBackendError) Unwrap() error { return e.Cause }

func (e SecretBackendError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.SecretBackendError",
		Text:   `secret backend error for "{{.Key}}": {{.Cause}}`,
		Hint:   "check your secrets backend configuration",
		Data:   e,
		Source: &e.Source,
	}
}

func (SecretBackendError) Severity() signal.Severity { return signal.Error }
func (SecretBackendError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

func (e *SecretBackendError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// SecretsConfigError is raised for invalid secrets() builtin usage.
type SecretsConfigError struct {
	Detail string
	Source spec.SourceSpan
}

func (e SecretsConfigError) Error() string {
	return fmt.Sprintf("secrets: %s", e.Detail)
}

func (e SecretsConfigError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.SecretsConfigError",
		Text:   "secrets: {{.Detail}}",
		Data:   e,
		Source: &e.Source,
	}
}

func (SecretsConfigError) Severity() signal.Severity { return signal.Error }
func (SecretsConfigError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

func (e *SecretsConfigError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// RemoteURLError is raised when remote() receives an invalid URL.
type RemoteURLError struct {
	URL    string
	Detail string
	Source spec.SourceSpan
}

func (e RemoteURLError) Error() string {
	return fmt.Sprintf("remote: %s", e.Detail)
}

func (e RemoteURLError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.RemoteURLError",
		Text:   "remote: {{.Detail}}",
		Hint:   "url must start with http:// or https://",
		Data:   e,
		Source: &e.Source,
	}
}

func (RemoteURLError) Severity() signal.Severity { return signal.Error }
func (RemoteURLError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

func (e *RemoteURLError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// RemoteChecksumError is raised when remote() receives a malformed checksum.
type RemoteChecksumError struct {
	Checksum string
	Detail   string
	Source   spec.SourceSpan
}

func (e RemoteChecksumError) Error() string {
	return fmt.Sprintf("remote: %s", e.Detail)
}

func (e RemoteChecksumError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.RemoteChecksumError",
		Text:   "remote: {{.Detail}}",
		Hint:   `checksum must be "algo:hex" (sha256, sha384, sha512, sha1, md5)`,
		Data:   e,
		Source: &e.Source,
	}
}

func (RemoteChecksumError) Severity() signal.Severity { return signal.Error }
func (RemoteChecksumError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

func (e *RemoteChecksumError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}

// PoisonValueError is raised when a declaration builtin's return value is used
// where a real value is expected (e.g. passed as a kwarg to a step).
type PoisonValueError struct {
	FuncName string
	Source   spec.SourceSpan
}

func (e PoisonValueError) Error() string {
	return fmt.Sprintf("%s() is a top-level declaration and cannot be used as a value", e.FuncName)
}

func (e PoisonValueError) EventTemplate() event.Template {
	return event.Template{
		ID:     "star.PoisonValue",
		Text:   "{{.FuncName}}() is a top-level declaration and cannot be used as a value",
		Hint:   "declaration builtins must be called at the top level, not nested inside other calls",
		Data:   e,
		Source: &e.Source,
	}
}

func (PoisonValueError) Severity() signal.Severity { return signal.Error }
func (PoisonValueError) Impact() diagnostic.Impact { return diagnostic.ImpactAbort }

func (e *PoisonValueError) setSource(s spec.SourceSpan) {
	if e.Source == (spec.SourceSpan{}) {
		e.Source = s
	}
}
