// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func main() {
	log := slogLog{slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := run(ctx, os.Args[1:], log)
	switch {
	case err == nil:
		return
	case errors.Is(err, ErrSnapshotRejected):
		log.Error(ctx, "snapshot rejected", "err", err)
		os.Exit(2)
	case errors.Is(err, ErrApplyFailed):
		log.Error(ctx, "apply failed", "err", err)
		os.Exit(1)
	default:
		log.Error(ctx, "scampi failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, log Log) error {
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "apply":
		return cmdApply(ctx, args[1:], log)
	case "run":
		return cmdRun(ctx, args[1:], log)
	default:
		return errUsage
	}
}

var errUsage = errors.New("usage: scampi {apply|run} <dir>")

// ErrSnapshotRejected wraps anything that makes the desired-state
// snapshot structurally unusable (parse, schema). Nothing applies.
var ErrSnapshotRejected = errors.New("snapshot rejected")

// ErrApplyFailed wraps per-resource runtime failures. Some resources
// may have landed; the failures aggregate.
var ErrApplyFailed = errors.New("apply failed")

func cmdApply(ctx context.Context, args []string, log Log) error {
	fset := flag.NewFlagSet("apply", flag.ContinueOnError)
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() != 1 {
		return errUsage
	}
	return reconcileOnce(ctx, fset.Arg(0), log)
}

func cmdRun(ctx context.Context, args []string, log Log) error {
	fset := flag.NewFlagSet("run", flag.ContinueOnError)
	interval := fset.Duration("interval", 5*time.Second, "poll interval between snapshots")
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() != 1 {
		return errUsage
	}
	return runLoop(ctx, fset.Arg(0), *interval, log)
}

// reconcileOnce runs one full parse + validate + resolve + apply pass.
// The four phases are wrapped in their respective sentinel buckets so
// callers can distinguish "config is bad" from "system misbehaved".
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
	if aerr := apply(ctx, sorted, log); aerr != nil {
		return fmt.Errorf("%w: %w", ErrApplyFailed, aerr)
	}
	return nil
}

// runLoop polls dir for changes and reconciles whenever the inputs
// hash differently from the previous tick. Errors during reconcile
// log and the loop continues; a snapshot reject does not exit the
// process (the operator gets to fix the config without restarting).
func runLoop(ctx context.Context, dir string, interval time.Duration, log Log) error {
	log.Info(ctx, "starting run loop", "dir", dir, "interval", interval)
	var lastRev string
	for {
		log.Info(ctx, "loop tick", "dir", dir, "interval", interval)
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
		log.Info(ctx, "no changes", "dir", dir, "rev", rev)
		select {
		case <-ctx.Done():
			log.Info(ctx, "shutting down")
			return nil
		case <-time.After(interval):
		}
	}
}

// hashDir returns a stable revision over the inputs (file basenames
// + contents). Changing any byte in any .hcl file changes the rev.
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
		// Hash basename + null + content so renames bump the rev.
		_, _ = h.Write([]byte(filepath.Base(p)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(content)
	}
	return hex.EncodeToString(h.Sum(nil)[:8]), nil
}

// logReconcileErr routes a reconcile failure to the right severity.
// Snapshot rejects are operator-actionable config faults; apply
// failures are runtime issues the operator may need to investigate.
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

// Log
// -----------------------------------------------------------------------------

// Log is the observability shape passed to apply paths. slog-backed
// today; a richer impl can replace it without changing call sites.
type Log interface {
	Debug(ctx context.Context, msg string, args ...any)
	Info(ctx context.Context, msg string, args ...any)
	Warn(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, args ...any)
}

type slogLog struct{ l *slog.Logger }

func (s slogLog) Debug(ctx context.Context, msg string, args ...any) {
	s.l.DebugContext(ctx, msg, args...)
}

func (s slogLog) Info(ctx context.Context, msg string, args ...any) {
	s.l.InfoContext(ctx, msg, args...)
}

func (s slogLog) Warn(ctx context.Context, msg string, args ...any) {
	s.l.WarnContext(ctx, msg, args...)
}

func (s slogLog) Error(ctx context.Context, msg string, args ...any) {
	s.l.ErrorContext(ctx, msg, args...)
}

// Resource
// -----------------------------------------------------------------------------

// Ref names a resource by Kind + Name. The HCL traversal
// kind.name.attr is the canonical surface form; this is the
// machine-side equivalent.
type Ref struct {
	Kind string
	Name string
}

func (r Ref) String() string { return r.Kind + "." + r.Name }

// Resource is one parsed top-level HCL block. Attrs holds the
// resolved-literal attrs; ref-bearing attrs live in exprs until the
// resolve phase folds them back into Attrs.
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
	log.Debug(ctx, "parsing", "block", block, "path", path)
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
			// Defer eval until upstream is resolved.
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

// refsFromExpr returns the unique kind.name refs that expr depends
// on. Anything beyond the kind.name prefix in a traversal stays with
// HCL, evaluated at resolve time.
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

// validate runs per-Kind schema checks across the whole snapshot
// plus a cross-resource check that every ref points to a declared
// resource. Aggregates so the operator sees every fault in one pass.
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

// hasAttr is true if the attr is declared, either as a resolved
// literal or as a ref-bearing expression.
func hasAttr(r Resource, name string) bool {
	if _, ok := r.Attrs[name]; ok {
		return true
	}
	_, ok := r.exprs[name]
	return ok
}

// Resolve
// -----------------------------------------------------------------------------

// resolved is a resource's attrs after the resolve phase folded any
// upstream references into literal strings.
type resolved struct {
	ref   Ref
	attrs map[string]string
}

// resolveStore tracks resolved resources in topo order so subsequent
// expressions can reference their attrs via kind.name.attr.
type resolveStore []resolved

func (s *resolveStore) put(ref Ref, attrs map[string]string) {
	*s = append(*s, resolved{ref: ref, attrs: attrs})
}

// evalContext shapes the store into HCL's kind.name.attr variable
// scope: vars[kind] is an object whose members are name -> object(attrs).
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

// ctyStringObject wraps a string map as a cty object of strings.
func ctyStringObject(attrs map[string]string) cty.Value {
	out := make(map[string]cty.Value, len(attrs))
	for k, v := range attrs {
		out[k] = cty.StringVal(v)
	}
	return cty.ObjectVal(out)
}

// resolve topo-sorts the snapshot then evaluates every ref-bearing
// attr against the store built up from upstream resources' attrs.
// Returns the resources in dependency order with all attrs resolved
// to literal strings. Cycles, unknown references, and type mismatches
// at eval time are snapshot-level faults.
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

// topoSort orders resources so dependencies come before dependents.
// Kahn's algorithm; ties preserve input order for stable output.
// Returns ErrSnapshotRejected-shaped error on cycles.
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

func apply(ctx context.Context, resources []Resource, log Log) error {
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
	// path and content are required; schema validation enforces this
	// before any apply runs.
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
	// path is required; schema validation enforces this before any
	// apply runs.
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
