// SPDX-License-Identifier: GPL-3.0-only

// Package engine reconciles desired-state HCL snapshots against the
// real filesystem. Apply runs once; Run polls and reconciles
// continuously.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Log
// -----------------------------------------------------------------------------

type Log interface {
	Debug(ctx context.Context, msg string, args ...any)
	Info(ctx context.Context, msg string, args ...any)
	Warn(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, args ...any)
}

// Errors
// -----------------------------------------------------------------------------

// ErrSnapshotRejected wraps any structural fault (parse, schema,
// ref, cycle). Nothing applies when it fires.
var ErrSnapshotRejected = errors.New("snapshot rejected")

// ErrApplyFailed wraps per-resource runtime failures. Some resources
// may have landed; failures aggregate.
var ErrApplyFailed = errors.New("apply failed")

// Public API
// -----------------------------------------------------------------------------

func Apply(ctx context.Context, dir string, log Log) error {
	return reconcileOnce(ctx, dir, log)
}

// Run polls dir and reconciles when the inputs change. Reconcile
// errors log and the loop continues; a snapshot reject does NOT exit
// the process so the operator can fix the config in place.
func Run(ctx context.Context, dir string, interval time.Duration, log Log) error {
	return runLoop(ctx, dir, interval, log)
}

// Pipeline
// -----------------------------------------------------------------------------

func reconcileOnce(ctx context.Context, dir string, log Log) error {
	resources, err := parseDir(ctx, log, dir)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
	}
	if verr := validate(resources); verr != nil {
		return fmt.Errorf("%w: %w", ErrSnapshotRejected, verr)
	}
	sorted, rerr := resolve(resources)
	if rerr != nil {
		return fmt.Errorf("%w: %w", ErrSnapshotRejected, rerr)
	}
	if aerr := applyAll(ctx, sorted, log); aerr != nil {
		return fmt.Errorf("%w: %w", ErrApplyFailed, aerr)
	}
	return nil
}

func runLoop(ctx context.Context, dir string, interval time.Duration, log Log) error {
	log.Info(ctx, "starting run loop", "dir", dir, "interval", interval)
	var lastRev string
	for {
		rev, hashErr := hashDir(dir)
		switch {
		case hashErr != nil:
			log.Error(ctx, "hash dir", "err", hashErr)
		case rev != lastRev:
			log.Debug(ctx, "snapshot change", "rev", rev)
			if rerr := reconcileOnce(ctx, dir, log); rerr != nil {
				logReconcileErr(ctx, log, rerr)
			}
			lastRev = rev
		}
		select {
		case <-ctx.Done():
			log.Info(ctx, "shutting down")
			return nil
		case <-time.After(interval):
		}
	}
}

func hashDir(dir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.hcl"))
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		content, rerr := os.ReadFile(p)
		if rerr != nil {
			return "", rerr
		}
		// basename + null + content so renames bump the rev
		_, _ = h.Write([]byte(filepath.Base(p)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(content)
	}
	return hex.EncodeToString(h.Sum(nil)[:8]), nil
}

func logReconcileErr(ctx context.Context, log Log, err error) {
	switch {
	case errors.Is(err, ErrSnapshotRejected):
		log.Warn(ctx, "snapshot rejected", "err", err)
	case errors.Is(err, ErrApplyFailed):
		log.Warn(ctx, "apply failed", "err", err)
	default:
		log.Error(ctx, "reconcile failed", "err", err)
	}
}

// Resource
// -----------------------------------------------------------------------------

type Ref struct {
	Kind string
	Name string
}

func (r Ref) String() string { return r.Kind + "." + r.Name }

// Resource is one parsed top-level HCL block. Attrs holds literal
// values resolved at parse time; ref-bearing attrs live in exprs
// until the resolve phase folds them back into Attrs.
type Resource struct {
	Kind  string
	Name  string
	Attrs map[string]string

	exprs map[string]hclsyntax.Expression
	deps  []Ref
}

func (r Resource) Ref() Ref { return Ref{Kind: r.Kind, Name: r.Name} }

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
		rs, perr := parseFile(ctx, log, p)
		if perr != nil {
			return nil, perr
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
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("%s: unexpected body type %T", path, file.Body)
	}
	if len(body.Attributes) > 0 {
		return nil, fmt.Errorf("%s: top-level attributes not allowed; use blocks", path)
	}
	out := make([]Resource, 0, len(body.Blocks))
	for _, block := range body.Blocks {
		r, perr := parseBlock(ctx, log, block, path)
		if perr != nil {
			return nil, perr
		}
		out = append(out, r)
	}
	return out, nil
}

func parseBlock(ctx context.Context, log Log, block *hclsyntax.Block, path string) (Resource, error) {
	log.Debug(ctx, "parsing", "block", block.Type, "path", path)
	if len(block.Labels) != 1 {
		return Resource{}, fmt.Errorf("%s: %s block needs exactly one label, got %d",
			path, block.Type, len(block.Labels))
	}
	if len(block.Body.Blocks) > 0 {
		return Resource{}, fmt.Errorf("%s: nested blocks not supported", path)
	}
	attrs := make(map[string]string, len(block.Body.Attributes))
	exprs := map[string]hclsyntax.Expression{}
	seenDeps := map[Ref]bool{}
	var deps []Ref
	for name, attr := range block.Body.Attributes {
		refs := refsFromExpr(attr.Expr)
		if len(refs) > 0 {
			exprs[name] = attr.Expr
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
		if val.Type() != cty.String {
			return Resource{}, fmt.Errorf("%s:%d: attr %q must be a string",
				path, attr.Range().Start.Line, name)
		}
		attrs[name] = val.AsString()
	}
	return Resource{
		Kind:  block.Type,
		Name:  block.Labels[0],
		Attrs: attrs,
		exprs: exprs,
		deps:  deps,
	}, nil
}

// refsFromExpr collects kind.name refs from a traversal. Anything
// past the kind.name prefix stays in the HCL expression and gets
// evaluated against the resolve store later.
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

// Validate
// -----------------------------------------------------------------------------

// validate aggregates so the operator sees every fault in one pass
// instead of trickling them in one reconcile at a time.
func validate(resources []Resource) error {
	var errs []error
	known := make(map[Ref]bool, len(resources))
	for _, r := range resources {
		known[r.Ref()] = true
	}
	for _, r := range resources {
		if err := validateOne(r); err != nil {
			errs = append(errs, err)
		}
		for _, dep := range r.deps {
			if !known[dep] {
				errs = append(errs, fmt.Errorf("%s: references unknown resource %q", r.Ref(), dep))
			}
		}
	}
	return errors.Join(errs...)
}

func validateOne(r Resource) error {
	switch r.Kind {
	case "file":
		return validateFile(r)
	case "dir":
		return validateDir(r)
	default:
		return fmt.Errorf("%s: unknown kind", r.Ref())
	}
}

func validateFile(r Resource) error {
	var errs []error
	if !hasAttr(r, "path") {
		errs = append(errs, fmt.Errorf("%s: missing required attr %q", r.Ref(), "path"))
	}
	if !hasAttr(r, "content") {
		errs = append(errs, fmt.Errorf("%s: missing required attr %q", r.Ref(), "content"))
	}
	return errors.Join(errs...)
}

func validateDir(r Resource) error {
	if !hasAttr(r, "path") {
		return fmt.Errorf("%s: missing required attr %q", r.Ref(), "path")
	}
	return nil
}

func hasAttr(r Resource, name string) bool {
	if _, ok := r.Attrs[name]; ok {
		return true
	}
	_, ok := r.exprs[name]
	return ok
}

// Resolve
// -----------------------------------------------------------------------------

type resolved struct {
	ref   Ref
	attrs map[string]string
}

type resolveStore []resolved

func (s *resolveStore) put(ref Ref, attrs map[string]string) {
	*s = append(*s, resolved{ref: ref, attrs: attrs})
}

// evalContext shapes the store into HCL's kind.name.attr scope:
// vars[kind] is an object whose members are name -> object(attrs).
func (s resolveStore) evalContext() *hcl.EvalContext {
	byKind := map[string]map[string]cty.Value{}
	for _, e := range s {
		if byKind[e.ref.Kind] == nil {
			byKind[e.ref.Kind] = map[string]cty.Value{}
		}
		byKind[e.ref.Kind][e.ref.Name] = ctyStringObject(e.attrs)
	}
	vars := make(map[string]cty.Value, len(byKind))
	for k, m := range byKind {
		vars[k] = cty.ObjectVal(m)
	}
	return &hcl.EvalContext{Variables: vars}
}

func ctyStringObject(attrs map[string]string) cty.Value {
	out := make(map[string]cty.Value, len(attrs))
	for k, v := range attrs {
		out[k] = cty.StringVal(v)
	}
	return cty.ObjectVal(out)
}

// resolve topo-sorts then folds ref-bearing attrs in dependency
// order. Cycles, unknown refs, and type mismatches at eval time are
// snapshot-level faults; runtime failure of an upstream apply is not.
func resolve(resources []Resource) ([]Resource, error) {
	sorted, err := topoSort(resources)
	if err != nil {
		return nil, err
	}
	var store resolveStore
	var errs []error
	for i := range sorted {
		r := &sorted[i]
		if len(r.exprs) > 0 {
			evalCtx := store.evalContext()
			for name, expr := range r.exprs {
				val, diags := expr.Value(evalCtx)
				if diags.HasErrors() {
					errs = append(errs, fmt.Errorf("%s: eval %q: %w", r.Ref(), name, diags))
					continue
				}
				if val.Type() != cty.String {
					errs = append(errs, fmt.Errorf("%s: attr %q must resolve to a string", r.Ref(), name))
					continue
				}
				r.Attrs[name] = val.AsString()
			}
		}
		store.put(r.Ref(), r.Attrs)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return sorted, nil
}

// topoSort uses Kahn's algorithm. Ties preserve input order so output
// is stable across runs.
func topoSort(resources []Resource) ([]Resource, error) {
	byRef := make(map[Ref]int, len(resources))
	for i, r := range resources {
		byRef[r.Ref()] = i
	}
	indeg := make([]int, len(resources))
	dependents := make(map[int][]int, len(resources))
	for i, r := range resources {
		for _, dep := range r.deps {
			j, ok := byRef[dep]
			if !ok {
				continue
			}
			indeg[i]++
			dependents[j] = append(dependents[j], i)
		}
	}
	queue := make([]int, 0, len(resources))
	for i, d := range indeg {
		if d == 0 {
			queue = append(queue, i)
		}
	}
	out := make([]Resource, 0, len(resources))
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		out = append(out, resources[i])
		for _, j := range dependents[i] {
			indeg[j]--
			if indeg[j] == 0 {
				queue = append(queue, j)
			}
		}
	}
	if len(out) != len(resources) {
		var cyclic []string
		for i, d := range indeg {
			if d > 0 {
				cyclic = append(cyclic, resources[i].Ref().String())
			}
		}
		sort.Strings(cyclic)
		return nil, fmt.Errorf("dependency cycle: %s", strings.Join(cyclic, ", "))
	}
	return out, nil
}

// Apply
// -----------------------------------------------------------------------------

func applyAll(ctx context.Context, resources []Resource, log Log) error {
	var errs []error
	for _, r := range resources {
		if err := applyOne(ctx, r, log); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func applyOne(ctx context.Context, r Resource, log Log) error {
	switch r.Kind {
	case "file":
		return applyFile(ctx, r, log)
	case "dir":
		return applyDir(ctx, r, log)
	default:
		return fmt.Errorf("%s: unknown kind", r.Ref())
	}
}

func applyFile(ctx context.Context, r Resource, log Log) error {
	// path + content guaranteed present by validate
	path := r.Attrs["path"]
	content := r.Attrs["content"]
	current, rerr := os.ReadFile(path)
	switch {
	case rerr == nil && string(current) == content:
		log.Debug(ctx, "file in sync", "ref", r.Ref(), "path", path)
		return nil
	case rerr != nil && !errors.Is(rerr, fs.ErrNotExist):
		return fmt.Errorf("%s: read %s: %w", r.Ref(), path, rerr)
	}
	log.Info(ctx, "writing file", "ref", r.Ref(), "path", path)
	if werr := os.WriteFile(path, []byte(content), 0o644); werr != nil {
		return fmt.Errorf("%s: write %s: %w", r.Ref(), path, werr)
	}
	return nil
}

func applyDir(ctx context.Context, r Resource, log Log) error {
	path := r.Attrs["path"]
	info, serr := os.Stat(path)
	switch {
	case serr == nil && info.IsDir():
		log.Debug(ctx, "dir in sync", "ref", r.Ref(), "path", path)
		return nil
	case serr == nil:
		return fmt.Errorf("%s: %s exists but is not a directory", r.Ref(), path)
	case !errors.Is(serr, fs.ErrNotExist):
		return fmt.Errorf("%s: stat %s: %w", r.Ref(), path, serr)
	}
	log.Info(ctx, "creating dir", "ref", r.Ref(), "path", path)
	if merr := os.MkdirAll(path, 0o755); merr != nil {
		return fmt.Errorf("%s: mkdir %s: %w", r.Ref(), path, merr)
	}
	return nil
}
