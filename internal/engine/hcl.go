// SPDX-License-Identifier: GPL-3.0-only

// HCL+cty types stay in this file; engine.go imports neither.

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Parse
// -----------------------------------------------------------------------------

func parseDir(ctx context.Context, log Log, dir string) ([]Resource, error) {
	log.Debug(ctx, "parsing", "dir", dir)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", dir)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.hcl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []Resource
	for _, p := range paths {
		rs, err := parseFile(ctx, log, p)
		if err != nil {
			return nil, err
		}
		out = append(out, rs...)
	}
	return out, nil
}

func parseFile(ctx context.Context, log Log, path string) ([]Resource, error) {
	log.Debug(ctx, "parsing", "path", path)
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, diags := hclsyntax.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, diags
	}
	if file == nil {
		// ParseConfig returns non-nil when HasErrors is false, but
		// CodeQL can't prove it; this clears the alert.
		return nil, fmt.Errorf("%s: hclsyntax returned nil file with no errors", path)
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("%s: unexpected body type %T", path, file.Body)
	}
	if len(body.Attributes) > 0 {
		return nil, fmt.Errorf("%s: top-level attributes not allowed; use blocks", path)
	}
	out := make([]Resource, 0, len(body.Blocks))
	for _, block := range body.Blocks {
		r, err := parseBlock(ctx, log, block, path)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func parseBlock(
	ctx context.Context,
	log Log,
	block *hclsyntax.Block,
	path string,
) (Resource, error) {
	log.Debug(ctx, "parsing", "block", block.Type, "path", path)
	if len(block.Labels) != 1 {
		return Resource{}, fmt.Errorf("%s: %s block needs exactly one label, got %d",
			path, block.Type, len(block.Labels))
	}
	if len(block.Body.Blocks) > 0 {
		return Resource{}, fmt.Errorf("%s: nested blocks not supported", path)
	}
	raw := map[string]rawValue{}
	pending := map[string]resolvable{}
	seenDeps := map[Ref]bool{}
	var deps []Ref
	for name, attr := range block.Body.Attributes {
		refs := refsFromExpr(attr.Expr)
		if len(refs) > 0 {
			pending[name] = hclResolvable{expr: attr.Expr}
			for _, r := range refs {
				if !seenDeps[r] {
					seenDeps[r] = true
					deps = append(deps, r)
				}
			}
			continue
		}
		val, diags := attr.Expr.Value(nil)
		if diags.HasErrors() {
			return Resource{}, diags
		}
		raw[name] = hclRaw{val: val}
	}
	return Resource{
		Kind:    block.Type,
		Name:    block.Labels[0],
		Attrs:   Attrs{},
		raw:     raw,
		pending: pending,
		deps:    deps,
	}, nil
}

type hclRaw struct{ val cty.Value }

func (h hclRaw) Coerce(target ValueKind) (Value, error) {
	switch target {
	case ValueString:
		if h.val.Type() != cty.String {
			return Value{}, fmt.Errorf("must be string, got %s", h.val.Type().FriendlyName())
		}
		return StringValue(h.val.AsString()), nil
	case ValueBool:
		if h.val.Type() != cty.Bool {
			return Value{}, fmt.Errorf("must be bool, got %s", h.val.Type().FriendlyName())
		}
		return BoolValue(h.val.True()), nil
	}
	return Value{}, fmt.Errorf("unsupported target type %s", target)
}

// refsFromExpr collects kind.name refs from an expression. Anything
// past the kind.name prefix stays in the expression for later eval.
func refsFromExpr(expr hclsyntax.Expression) []Ref {
	seen := map[Ref]bool{}
	var refs []Ref
	for _, trav := range expr.Variables() {
		r, ok := traversalToRef(trav)
		if !ok {
			continue
		}
		if !seen[r] {
			seen[r] = true
			refs = append(refs, r)
		}
	}
	return refs
}

func traversalToRef(trav hcl.Traversal) (Ref, bool) {
	if len(trav) < 2 {
		return Ref{}, false
	}
	root, ok := trav[0].(hcl.TraverseRoot)
	if !ok {
		return Ref{}, false
	}
	attr, ok := trav[1].(hcl.TraverseAttr)
	if !ok {
		return Ref{}, false
	}
	return Ref{Kind: root.Name, Name: attr.Name}, true
}

// Resolvable impl
// -----------------------------------------------------------------------------

type hclResolvable struct {
	expr hclsyntax.Expression
}

func (h hclResolvable) Resolve(store []resolvedRef) (string, error) {
	ctx := buildEvalContext(store)
	val, diags := h.expr.Value(ctx)
	if diags.HasErrors() {
		return "", diags
	}
	if val.Type() != cty.String {
		return "", fmt.Errorf("must resolve to a string")
	}
	return val.AsString(), nil
}

// buildEvalContext shapes the store into HCL's kind.name.attr scope.
func buildEvalContext(store []resolvedRef) *hcl.EvalContext {
	byKind := map[string]map[string]cty.Value{}
	for _, e := range store {
		if byKind[e.Ref.Kind] == nil {
			byKind[e.Ref.Kind] = map[string]cty.Value{}
		}
		byKind[e.Ref.Kind][e.Ref.Name] = ctyStringObject(e.Attrs)
	}
	vars := make(map[string]cty.Value, len(byKind))
	for k, m := range byKind {
		vars[k] = cty.ObjectVal(m)
	}
	return &hcl.EvalContext{Variables: vars}
}

func ctyStringObject(attrs Attrs) cty.Value {
	out := make(map[string]cty.Value, len(attrs))
	for k, v := range attrs {
		out[k] = cty.StringVal(v.Str)
	}
	return cty.ObjectVal(out)
}
