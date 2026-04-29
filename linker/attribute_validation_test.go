// SPDX-License-Identifier: GPL-3.0-only

package linker

import (
	"testing"

	"scampi.dev/scampi/lang/ast"
	"scampi.dev/scampi/lang/token"
	"scampi.dev/scampi/spec"
)

// White-box tests for the validation-style attribute behaviours
// shipped in std/std.scampi: @nonempty, @filemode, @pattern, @oneof,
// @path. Each test runs StaticCheck with a synthetic
// StaticCheckContext and asserts the diagnostic count.
//
// Mirrors the pattern in attribute_secretkey_test.go. The point is
// fast, focused coverage of the contract the linker relies on after
// step-side validation was deleted in #166 — if any of these
// behaviours regresses, the migration's safety net is gone.

func newAttrCtx(name, paramName string, arg ast.Expr, args map[string]any) StaticCheckContext {
	return StaticCheckContext{
		Linker:    &linkContext{},
		AttrName:  name,
		AttrArgs:  args,
		ParamName: paramName,
		ParamArg:  arg,
		UseSpan:   spec.SourceSpan{},
	}
}

func diags(ctx StaticCheckContext) int {
	return len(ctx.Linker.(*linkContext).diags)
}

// listLitExpr builds a list literal AST node from a slice of items.
func listLitExpr(items ...ast.Expr) *ast.ListLit {
	return &ast.ListLit{Items: items, SrcSpan: token.Span{Start: 0, End: 1}}
}

// computedExpr returns a non-literal expression (an identifier
// reference) so we can test that behaviours skip dynamic args.
func computedExpr() ast.Expr {
	return &ast.Ident{Name: "var", SrcSpan: token.Span{Start: 0, End: 3}}
}

// NonEmpty
// -----------------------------------------------------------------------------

func TestNonEmpty_StringLiteralEmpty(t *testing.T) {
	ctx := newAttrCtx("std.@nonempty", "name", stringLitExpr(""), nil)
	NonEmptyAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 1 {
		t.Errorf("expected 1 diagnostic for empty string, got %d", diags(ctx))
	}
}

func TestNonEmpty_StringLiteralNonEmpty(t *testing.T) {
	ctx := newAttrCtx("std.@nonempty", "name", stringLitExpr("nginx"), nil)
	NonEmptyAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for non-empty string, got %d", diags(ctx))
	}
}

func TestNonEmpty_ListLiteralEmpty(t *testing.T) {
	ctx := newAttrCtx("std.@nonempty", "packages", listLitExpr(), nil)
	NonEmptyAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 1 {
		t.Errorf("expected 1 diagnostic for empty list, got %d", diags(ctx))
	}
}

func TestNonEmpty_ListLiteralNonEmpty(t *testing.T) {
	pkgs := listLitExpr(stringLitExpr("nginx"), stringLitExpr("certbot"))
	ctx := newAttrCtx("std.@nonempty", "packages", pkgs, nil)
	NonEmptyAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for non-empty list, got %d", diags(ctx))
	}
}

func TestNonEmpty_ComputedSkipped(t *testing.T) {
	ctx := newAttrCtx("std.@nonempty", "name", computedExpr(), nil)
	NonEmptyAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for computed expr, got %d", diags(ctx))
	}
}

// FileMode
// -----------------------------------------------------------------------------

func TestFileMode_ValidOctal(t *testing.T) {
	for _, perm := range []string{"0644", "0755", "0600", "rw-r--r--", "u=rw,g=r,o=r"} {
		ctx := newAttrCtx("std.@filemode", "perm", stringLitExpr(perm), nil)
		FileModeAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 0 {
			t.Errorf("expected no diagnostics for %q, got %d", perm, diags(ctx))
		}
	}
}

func TestFileMode_Invalid(t *testing.T) {
	for _, perm := range []string{"yolo", "9999", "rwxrwxrwxrwx", ""} {
		ctx := newAttrCtx("std.@filemode", "perm", stringLitExpr(perm), nil)
		FileModeAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 1 {
			t.Errorf("expected 1 diagnostic for %q, got %d", perm, diags(ctx))
		}
	}
}

func TestFileMode_ComputedSkipped(t *testing.T) {
	ctx := newAttrCtx("std.@filemode", "perm", computedExpr(), nil)
	FileModeAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for computed expr, got %d", diags(ctx))
	}
}

// Size
// -----------------------------------------------------------------------------

func TestSize_Valid(t *testing.T) {
	for _, s := range []string{"1024", "512K", "4M", "8G", "12T", "1.5G", "0.25T", "256B"} {
		ctx := newAttrCtx("std.@size", "memory", stringLitExpr(s), nil)
		SizeAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 0 {
			t.Errorf("expected no diagnostics for %q, got %d", s, diags(ctx))
		}
	}
}

func TestSize_Invalid(t *testing.T) {
	for _, s := range []string{"", "12g", "8 G", "M", "abc", "12X", "1..5G", "-1G", "+1G"} {
		ctx := newAttrCtx("std.@size", "memory", stringLitExpr(s), nil)
		SizeAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 1 {
			t.Errorf("expected 1 diagnostic for %q, got %d", s, diags(ctx))
		}
	}
}

func TestSize_ComputedSkipped(t *testing.T) {
	ctx := newAttrCtx("std.@size", "memory", computedExpr(), nil)
	SizeAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for computed expr, got %d", diags(ctx))
	}
}

// Pattern
// -----------------------------------------------------------------------------

func TestPattern_Match(t *testing.T) {
	args := map[string]any{"regex": "^[0-9]+(-[0-9]+)?(/(tcp|udp))?$"}
	for _, port := range []string{"22", "80/tcp", "8000-9000/udp"} {
		ctx := newAttrCtx("std.@pattern", "port", stringLitExpr(port), args)
		PatternAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 0 {
			t.Errorf("expected no diagnostics for %q, got %d", port, diags(ctx))
		}
	}
}

func TestPattern_NoMatch(t *testing.T) {
	args := map[string]any{"regex": "^[0-9]+(-[0-9]+)?(/(tcp|udp))?$"}
	for _, port := range []string{"abc", "22/sctp", "/tcp"} {
		ctx := newAttrCtx("std.@pattern", "port", stringLitExpr(port), args)
		PatternAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 1 {
			t.Errorf("expected 1 diagnostic for %q, got %d", port, diags(ctx))
		}
	}
}

func TestPattern_VerifyExactlyOnePercentS(t *testing.T) {
	// The regex shipped on copy.verify and template.verify:
	// requires exactly one %s, allows other %X tokens. The runtime
	// uses strings.Replace(verifyCmd, "%s", tmpFile, 1) — only the
	// first %s is substituted, so multi-%s would silently misbehave.
	// %% and %X (X != s) are fine because the shell sees them as
	// literal characters, not printf format directives.
	args := map[string]any{"regex": `^([^%]|%[^s])*%s([^%]|%[^s])*$`}
	cases := []struct {
		input string
		want  int
	}{
		{`visudo -cf %s`, 0},
		{`nginx -t -c %s`, 0},
		{`echo %d %s`, 0},         // %d is allowed (only %s is special)
		{`grep "100%" %s`, 0},     // % followed by " is allowed
		{`echo 100%% done %s`, 0}, // %% is allowed (literal in shell)
		{`%s`, 0},                 // bare %s
		{`visudo -cf`, 1},         // no %s
		{`diff %s %s`, 1},         // two %s — the bug we're guarding against
		{`%s %s %s`, 1},           // three %s
	}
	for _, c := range cases {
		ctx := newAttrCtx("std.@pattern", "verify", stringLitExpr(c.input), args)
		PatternAttribute{}.StaticCheck(ctx)
		if diags(ctx) != c.want {
			t.Errorf("verify=%q: expected %d diagnostics, got %d", c.input, c.want, diags(ctx))
		}
	}
}

func TestPattern_BadRegexFails(t *testing.T) {
	args := map[string]any{"regex": "[invalid"}
	ctx := newAttrCtx("std.@pattern", "x", stringLitExpr("anything"), args)
	PatternAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 1 {
		t.Errorf("expected 1 diagnostic for invalid regex, got %d", diags(ctx))
	}
}

func TestPattern_ComputedSkipped(t *testing.T) {
	args := map[string]any{"regex": "^.+$"}
	ctx := newAttrCtx("std.@pattern", "x", computedExpr(), args)
	PatternAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for computed expr, got %d", diags(ctx))
	}
}

// OneOf
// -----------------------------------------------------------------------------

func TestOneOf_Allowed(t *testing.T) {
	args := map[string]any{"values": []any{"present", "absent", "latest"}}
	for _, v := range []string{"present", "absent", "latest"} {
		ctx := newAttrCtx("std.@oneof", "state", stringLitExpr(v), args)
		OneOfAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 0 {
			t.Errorf("expected no diagnostics for %q, got %d", v, diags(ctx))
		}
	}
}

func TestOneOf_NotAllowed(t *testing.T) {
	args := map[string]any{"values": []any{"present", "absent", "latest"}}
	for _, v := range []string{"yolo", "Present", "", "PRESENT"} {
		ctx := newAttrCtx("std.@oneof", "state", stringLitExpr(v), args)
		OneOfAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 1 {
			t.Errorf("expected 1 diagnostic for %q, got %d", v, diags(ctx))
		}
	}
}

func TestOneOf_ComputedSkipped(t *testing.T) {
	args := map[string]any{"values": []any{"a", "b"}}
	ctx := newAttrCtx("std.@oneof", "x", computedExpr(), args)
	OneOfAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for computed expr, got %d", diags(ctx))
	}
}

// Path
// -----------------------------------------------------------------------------

func TestPath_AbsoluteRequired_Absolute(t *testing.T) {
	args := map[string]any{"absolute": true}
	for _, p := range []string{"/etc/nginx", "/", "/var/log/app.log"} {
		ctx := newAttrCtx("std.@path", "dest", stringLitExpr(p), args)
		PathAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 0 {
			t.Errorf("expected no diagnostics for %q, got %d", p, diags(ctx))
		}
	}
}

func TestPath_AbsoluteRequired_Relative(t *testing.T) {
	args := map[string]any{"absolute": true}
	for _, p := range []string{"etc/nginx", "./relative", "../up"} {
		ctx := newAttrCtx("std.@path", "dest", stringLitExpr(p), args)
		PathAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 1 {
			t.Errorf("expected 1 diagnostic for %q, got %d", p, diags(ctx))
		}
	}
}

func TestPath_RelativeAllowedByDefault(t *testing.T) {
	// Without absolute=true, relative paths are accepted.
	for _, p := range []string{"etc/nginx", "./relative", "/absolute"} {
		ctx := newAttrCtx("std.@path", "dest", stringLitExpr(p), nil)
		PathAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 0 {
			t.Errorf("expected no diagnostics for %q (no absolute=true), got %d", p, diags(ctx))
		}
	}
}

func TestPath_EmptyRejected(t *testing.T) {
	// Empty path is always rejected, regardless of the absolute arg.
	for _, args := range []map[string]any{nil, {"absolute": true}, {"absolute": false}} {
		ctx := newAttrCtx("std.@path", "dest", stringLitExpr(""), args)
		PathAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 1 {
			t.Errorf("expected 1 diagnostic for empty path with args=%v, got %d", args, diags(ctx))
		}
	}
}

func TestPath_NULRejected(t *testing.T) {
	ctx := newAttrCtx("std.@path", "dest", stringLitExpr("/etc/\x00bad"), nil)
	PathAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 1 {
		t.Errorf("expected 1 diagnostic for NUL byte, got %d", diags(ctx))
	}
}

func TestPath_ComputedSkipped(t *testing.T) {
	ctx := newAttrCtx("std.@path", "dest", computedExpr(), map[string]any{"absolute": true})
	PathAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for computed expr, got %d", diags(ctx))
	}
}

// Deprecated and Since
// -----------------------------------------------------------------------------

func TestDeprecated_AlwaysWarns(t *testing.T) {
	args := map[string]any{"message": "use foo() instead"}
	ctx := newAttrCtx("std.@deprecated", "old_field", stringLitExpr("anything"), args)
	DeprecatedAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 1 {
		t.Errorf("expected 1 warning for any use, got %d", diags(ctx))
	}
}

func TestDeprecated_NoMessageStillWarns(t *testing.T) {
	ctx := newAttrCtx("std.@deprecated", "old_field", stringLitExpr("anything"), nil)
	DeprecatedAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 1 {
		t.Errorf("expected 1 warning even without message, got %d", diags(ctx))
	}
}

func TestSince_Noop(t *testing.T) {
	args := map[string]any{"version": "0.5"}
	ctx := newAttrCtx("std.@since", "x", stringLitExpr("anything"), args)
	SinceAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", diags(ctx))
	}
}

// Min and Max
// -----------------------------------------------------------------------------

func intLitExpr(value int64) *ast.IntLit {
	return &ast.IntLit{Value: value, SrcSpan: token.Span{Start: 0, End: 5}}
}

func TestMin_InRange(t *testing.T) {
	args := map[string]any{"value": int64(1)}
	for _, v := range []int64{1, 100, 65535} {
		ctx := newAttrCtx("std.@min", "port", intLitExpr(v), args)
		MinAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 0 {
			t.Errorf("expected no diagnostics for %d, got %d", v, diags(ctx))
		}
	}
}

func TestMin_BelowRange(t *testing.T) {
	args := map[string]any{"value": int64(1)}
	for _, v := range []int64{0, -1, -100} {
		ctx := newAttrCtx("std.@min", "port", intLitExpr(v), args)
		MinAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 1 {
			t.Errorf("expected 1 diagnostic for %d, got %d", v, diags(ctx))
		}
	}
}

func TestMin_ComputedSkipped(t *testing.T) {
	args := map[string]any{"value": int64(1)}
	ctx := newAttrCtx("std.@min", "port", computedExpr(), args)
	MinAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for computed expr, got %d", diags(ctx))
	}
}

func TestMax_InRange(t *testing.T) {
	args := map[string]any{"value": int64(65535)}
	for _, v := range []int64{1, 100, 65535} {
		ctx := newAttrCtx("std.@max", "port", intLitExpr(v), args)
		MaxAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 0 {
			t.Errorf("expected no diagnostics for %d, got %d", v, diags(ctx))
		}
	}
}

func TestMax_AboveRange(t *testing.T) {
	args := map[string]any{"value": int64(65535)}
	for _, v := range []int64{65536, 70000, 100000} {
		ctx := newAttrCtx("std.@max", "port", intLitExpr(v), args)
		MaxAttribute{}.StaticCheck(ctx)
		if diags(ctx) != 1 {
			t.Errorf("expected 1 diagnostic for %d, got %d", v, diags(ctx))
		}
	}
}

func TestMax_ComputedSkipped(t *testing.T) {
	args := map[string]any{"value": int64(65535)}
	ctx := newAttrCtx("std.@max", "port", computedExpr(), args)
	MaxAttribute{}.StaticCheck(ctx)
	if diags(ctx) != 0 {
		t.Errorf("expected no diagnostics for computed expr, got %d", diags(ctx))
	}
}
