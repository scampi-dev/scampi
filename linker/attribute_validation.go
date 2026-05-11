// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/perm"
	"scampi.dev/scampi/spec"
)

// This file collects the small validation-style attribute behaviours
// shipped with scampi: @nonempty, @filemode, @pattern, @oneof,
// @deprecated, @since, @path. Each behaviour validates literal
// arguments at link time and emits a typed diagnostic when they
// don't satisfy the rule. Non-literal arguments fall through to the
// runtime — the lang itself doesn't have arbitrary value checking,
// so anything dynamic just runs as the program intends and any
// failures surface from the actual op when it executes.

// literalString extracts a literal string value for an annotated
// param. Prefers ctx.Resolved (eval-walk path) over ctx.ParamArg
// (AST-walk path). Returns (value, true) when a string can be
// recovered; (\"\", false) for any other shape (computed strings,
// non-string types, missing both inputs). The boolean lets behaviours
// distinguish "literal empty" from "non-literal expression".
func literalString(ctx StaticCheckContext) (string, bool) {
	if s, ok := ctx.Resolved.(string); ok {
		return s, true
	}
	sl, ok := ctx.ParamArg.(*ast.StringLit)
	if !ok || len(sl.Parts) != 1 {
		return "", false
	}
	text, ok := sl.Parts[0].(*ast.StringText)
	if !ok {
		return "", false
	}
	return text.Raw, true
}

// literalList reports the length of a list-shaped annotated param.
// Prefers ctx.Resolved (any Go slice via reflection) over a literal
// AST list. Returns (length, true) when a list shape is recognised.
func literalList(ctx StaticCheckContext) (int, bool) {
	if ctx.Resolved != nil {
		rv := reflect.ValueOf(ctx.Resolved)
		if rv.IsValid() && rv.Kind() == reflect.Slice {
			return rv.Len(), true
		}
	}
	if list, ok := ctx.ParamArg.(*ast.ListLit); ok {
		return len(list.Items), true
	}
	return 0, false
}

// NonEmptyAttribute fails when the annotated parameter binds to a
// literal empty value. Handles both string and list literals — the
// "must not be empty" rule reads identically for either shape, and
// list-typed params (e.g. pkg.packages) deserve the same fast-fail
// treatment as string-typed ones. Hint and Help come from the
// attribute type's doc comment.
type NonEmptyAttribute struct{}

func (NonEmptyAttribute) StaticCheck(ctx StaticCheckContext) {
	if v, ok := literalString(ctx); ok {
		if v == "" {
			ctx.Linker.Emit(newAttrDocError(
				ctx,
				fmt.Sprintf("%s must not be empty", ctx.ParamName),
			))
		}
		return
	}
	if n, ok := literalList(ctx); ok && n == 0 {
		ctx.Linker.Emit(newAttrDocError(
			ctx,
			fmt.Sprintf("%s must not be empty", ctx.ParamName),
		))
	}
}

// FileModeAttribute validates that the annotated string parameter is
// a recognised file permission literal. Delegates parsing to
// perm.ParsePerm so the static check accepts the same formats as
// the runtime check (octal, ls-style, posix style). The error
// rendering pulls Hint and Help from the attribute type's doc
// comment in std/std.scampi — single source of truth.
type FileModeAttribute struct{}

func (FileModeAttribute) StaticCheck(ctx StaticCheckContext) {
	v, ok := literalString(ctx)
	if !ok {
		return
	}
	if _, err := perm.ParsePerm(v, ctx.UseSpan); err != nil {
		ctx.Linker.Emit(newAttrDocError(ctx, fmt.Sprintf("invalid file permission %q", v)))
	}
}

// SizeAttribute validates that the annotated string parameter is a
// recognised human-readable byte amount. Accepts bare integers (bytes)
// or numbers with an uppercase unit suffix B/K/M/G/T, optionally with
// a decimal point. Lowercase suffixes are rejected — keeping it case-
// sensitive avoids ambiguity in mixed-case configs.
type SizeAttribute struct{}

var sizeRegex = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?[BKMGT]?$`)

func (SizeAttribute) StaticCheck(ctx StaticCheckContext) {
	v, ok := literalString(ctx)
	if !ok {
		return
	}
	if !sizeRegex.MatchString(v) {
		ctx.Linker.Emit(newAttrDocError(ctx, fmt.Sprintf("invalid size %q", v)))
	}
}

// PatternAttribute validates that the annotated string parameter
// matches a regex declared on the attribute itself (the `regex`
// arg). The error includes the regex itself for context; the
// attribute type's doc comment provides any additional Hint/Help.
type PatternAttribute struct{}

func (PatternAttribute) StaticCheck(ctx StaticCheckContext) {
	v, ok := literalString(ctx)
	if !ok {
		return
	}
	rawRegex, _ := ctx.AttrArgs["regex"].(string)
	if rawRegex == "" {
		return
	}
	re, err := regexp.Compile(rawRegex)
	if err != nil {
		// Bad regex on the attribute itself — surface as a fatal so
		// stub authors notice.
		ctx.Linker.Emit(newAttrDocError(
			ctx,
			fmt.Sprintf("invalid pattern %q on attribute: %s", rawRegex, err),
		))
		return
	}
	if !re.MatchString(v) {
		ctx.Linker.Emit(newAttrDocError(
			ctx,
			fmt.Sprintf("%q does not match pattern %s", v, rawRegex),
		))
	}
}

// OneOfAttribute validates that the annotated string parameter is one
// of the values declared on the attribute itself (the `values` list).
// The accepted set is rendered inline in the error message; any
// additional Hint/Help comes from the attribute type's doc comment.
type OneOfAttribute struct{}

func (OneOfAttribute) StaticCheck(ctx StaticCheckContext) {
	v, ok := literalString(ctx)
	if !ok {
		return
	}
	values, _ := ctx.AttrArgs["values"].([]any)
	if len(values) == 0 {
		return
	}
	for _, allowed := range values {
		if s, ok := allowed.(string); ok && s == v {
			return
		}
	}
	want := make([]string, 0, len(values))
	for _, a := range values {
		if s, ok := a.(string); ok {
			want = append(want, fmt.Sprintf("%q", s))
		}
	}
	ctx.Linker.Emit(newAttrDocError(
		ctx,
		fmt.Sprintf("%q is not allowed; must be one of: %s", v, strings.Join(want, ", ")),
	))
}

// DeprecatedAttribute emits a warning diagnostic at every use of an
// annotated parameter. The optional `message` arg is rendered inline.
type DeprecatedAttribute struct{}

func (DeprecatedAttribute) StaticCheck(ctx StaticCheckContext) {
	msg, _ := ctx.AttrArgs["message"].(string)
	ctx.Linker.Emit(&attrDeprecationWarning{
		Param:   ctx.ParamName,
		Attr:    ctx.AttrName,
		Message: msg,
		Src:     &ctx.UseSpan,
	})
}

// SinceAttribute is purely informational and emits no diagnostics.
// The version is surfaced through hover docs (LSP-side) only.
type SinceAttribute struct{}

func (SinceAttribute) StaticCheck(_ StaticCheckContext) {}

// literalInt extracts a literal int64 for an annotated param. Prefers
// ctx.Resolved (any concrete int kind) over ctx.ParamArg AST literal.
func literalInt(ctx StaticCheckContext) (int64, bool) {
	if ctx.Resolved != nil {
		rv := reflect.ValueOf(ctx.Resolved)
		if rv.IsValid() {
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return rv.Int(), true
			}
		}
	}
	il, ok := ctx.ParamArg.(*ast.IntLit)
	if !ok {
		return 0, false
	}
	return il.Value, true
}

// MinAttribute validates that the annotated integer parameter is at
// least the given minimum. Only fires on literal int arguments.
type MinAttribute struct{}

func (MinAttribute) StaticCheck(ctx StaticCheckContext) {
	v, ok := literalInt(ctx)
	if !ok {
		return
	}
	bound, _ := ctx.AttrArgs["value"].(int64)
	if v < bound {
		ctx.Linker.Emit(newAttrDocError(
			ctx,
			fmt.Sprintf("%s = %d is below minimum %d", ctx.ParamName, v, bound),
		))
	}
}

// MaxAttribute validates that the annotated integer parameter is at
// most the given maximum. Only fires on literal int arguments.
type MaxAttribute struct{}

func (MaxAttribute) StaticCheck(ctx StaticCheckContext) {
	v, ok := literalInt(ctx)
	if !ok {
		return
	}
	bound, _ := ctx.AttrArgs["value"].(int64)
	if v > bound {
		ctx.Linker.Emit(newAttrDocError(
			ctx,
			fmt.Sprintf("%s = %d exceeds maximum %d", ctx.ParamName, v, bound),
		))
	}
}

// PathAttribute validates that the annotated string parameter looks
// like a filesystem path. v1 checks the most basic shape: non-empty,
// no NUL bytes. The optional `absolute` arg requires the path to
// begin with `/`. The `must_exist` arg is reserved for a future
// source-side existence check.
type PathAttribute struct{}

func (PathAttribute) StaticCheck(ctx StaticCheckContext) {
	v, ok := literalString(ctx)
	if !ok {
		return
	}
	if v == "" {
		ctx.Linker.Emit(newAttrDocError(ctx, "path must not be empty"))
		return
	}
	if strings.ContainsRune(v, 0) {
		ctx.Linker.Emit(newAttrDocError(ctx, "path must not contain NUL bytes"))
		return
	}
	absolute, _ := ctx.AttrArgs["absolute"].(bool)
	if absolute && !strings.HasPrefix(v, "/") {
		ctx.Linker.Emit(newAttrDocError(
			ctx,
			fmt.Sprintf("%q must be an absolute path (start with \"/\")", v),
		))
	}
}

// newAttrDocError builds an attrDocError from a StaticCheckContext.
// The error's Hint and Help are derived from the attribute type's
// doc comment via splitDoc — single source of truth lives in the
// `type @name { ... }` declaration in std/std.scampi.
func newAttrDocError(ctx StaticCheckContext, message string) *attrDocError {
	hint, help := splitDoc(ctx.AttrDoc)
	return &attrDocError{
		Param:   ctx.ParamName,
		Attr:    ctx.AttrName,
		Message: message,
		Hint:    hint,
		Help:    help,
		Src:     &ctx.UseSpan,
	}
}

// splitDoc splits an attribute type's doc comment into Hint (the
// first paragraph) and Help (everything after the first blank line).
// Empty input returns ("", ""); a single-paragraph doc returns
// (paragraph, "").
func splitDoc(doc string) (hint, help string) {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return "", ""
	}
	if before, after, ok := strings.Cut(doc, "\n\n"); ok {
		return strings.TrimSpace(before), strings.TrimSpace(after)
	}
	return doc, ""
}

// attrDocError is the rich error shape for validation-style attribute
// behaviours that derive their Hint and Help from the attribute
// type's doc comment in the source. The Text is the behaviour's own
// message; the rest is data-driven by the doc comment so stub
// authors maintain the UX in one place.
type attrDocError struct {
	diagnostic.FatalError
	Param   string
	Attr    string
	Message string
	Hint    string
	Help    string
	Src     *spec.SourceSpan
}

func (e *attrDocError) Error() string {
	return e.Message
}

func (e *attrDocError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeAttributeViolation,
		Text:   "{{.Message}}",
		Hint:   "{{.Hint}}",
		Help:   "{{.Help}}",
		Source: e.Src,
		Data: attrDocErrorData{
			Param:   e.Param,
			Attr:    e.Attr,
			Message: e.Message,
			Hint:    e.Hint,
			Help:    e.Help,
		},
	}
}

type attrDocErrorData struct {
	Param   string
	Attr    string
	Message string
	Hint    string
	Help    string
}

// attrDeprecationWarning is a non-fatal diagnostic for `@deprecated`
// usage. Severity is Warning so the engine doesn't abort.
type attrDeprecationWarning struct {
	diagnostic.Warning
	Param   string
	Attr    string
	Message string
	Src     *spec.SourceSpan
}

func (e *attrDeprecationWarning) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s is deprecated: %s", e.Param, e.Message)
	}
	return fmt.Sprintf("%s is deprecated", e.Param)
}

func (e *attrDeprecationWarning) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeAttributeDeprecated,
		Text:   "{{.Param}} is deprecated{{if .Message}}: {{.Message}}{{end}}",
		Source: e.Src,
		Data: attrDeprecationData{
			Param:   e.Param,
			Message: e.Message,
		},
	}
}

type attrDeprecationData struct {
	Param   string
	Message string
}
