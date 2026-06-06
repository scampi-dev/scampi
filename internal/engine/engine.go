// SPDX-License-Identifier: GPL-3.0-only

// Package engine reconciles desired state against live observation.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Errors
// -----------------------------------------------------------------------------

// ErrSnapshotRejected wraps any structural fault (parse, schema,
// ref, cycle). Nothing applies when it fires.
var ErrSnapshotRejected = errors.New("snapshot rejected")

// ErrReconcileFailed wraps per-resource runtime failures. Some
// resources may have landed; failures aggregate.
var ErrReconcileFailed = errors.New("reconcile failed")

// openSinkAndInventory opens the action log writer rooted at dir
// and resolves the starting inventory. inv is used as-is if
// non-nil; otherwise it's loaded (strict replay) from dir.
func openSinkAndInventory(dir string, inv *Inventory) (*Inventory, *ActionLog, error) {
	if inv == nil {
		loaded, err := LoadInventory(dir)
		if err != nil {
			return nil, nil, fmt.Errorf("action log replay: %w", err)
		}
		inv = loaded
	}
	actLog, err := NewActionLog(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("action log: %w", err)
	}
	return inv, actLog, nil
}

// Pipeline
// -----------------------------------------------------------------------------

// snapshot parses, validates, and resolves dir. All faults bucket
// as ErrSnapshotRejected.
func snapshot(ctx context.Context, dir string, log Log) ([]Resource, error) {
	resources, err := parseDir(ctx, log, dir)
	if err != nil {
		return nil, rejected(ctx, log, "parse", err)
	}
	if err := typecheck(resources); err != nil {
		return nil, rejected(ctx, log, "typecheck", err)
	}
	if err := analyze(resources); err != nil {
		return nil, rejected(ctx, log, "analyze", err)
	}
	sorted, err := resolve(resources)
	if err != nil {
		return nil, rejected(ctx, log, "resolve", err)
	}
	if err := verify(sorted); err != nil {
		return nil, rejected(ctx, log, "verify", err)
	}
	log.Emit(ctx, CodeSnapshotReceived, nil, "resources", len(sorted))
	return sorted, nil
}

func rejected(ctx context.Context, log Log, phase string, err error) error {
	log.Emit(ctx, CodeSnapshotRejected, nil, "phase", phase, "err", err)
	return fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
}

func hashDir(dir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.hcl"))
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		// basename + NUL + content so renames bump the rev
		_, _ = h.Write([]byte(filepath.Base(p)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(content)
	}
	return hex.EncodeToString(h.Sum(nil)[:8]), nil
}

func logReconcileErr(ctx context.Context, log Log, err error) {
	switch {
	case errors.Is(err, ErrSnapshotRejected):
		// rejected() already emitted CodeSnapshotRejected at the
		// rejection site; logging here would double-report.
	case errors.Is(err, ErrReconcileFailed):
		// Per-resource apply.failed events already convey the
		// failure with detail; an aggregated warn is redundant.
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

type Attrs map[string]Value

func (a Attrs) GetString(name string) string { return a[name].Str }
func (a Attrs) GetBool(name string) bool     { return a[name].Bool }

type Position struct {
	Path string
	Line int
	Col  int
}

func (p Position) String() string {
	if p.Path == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", p.Path, p.Line, p.Col)
}

// prefix returns "Position: " or "" when empty, so error sites can
// drop the prefix uniformly when no source is known (in-memory tests).
func (p Position) prefix() string {
	if p.Path == "" {
		return ""
	}
	return p.String() + ": "
}

type Resource struct {
	Kind  string
	Name  string
	Attrs Attrs

	Source      Position
	AttrSources map[string]Position
	depSources  map[Ref]Position

	// raw holds cty values from parse until typecheck coerces them.
	raw     map[string]rawValue
	pending map[string]resolvable
	deps    []Ref
}

func (r Resource) Ref() Ref { return Ref{Kind: r.Kind, Name: r.Name} }

// Has reports presence across all attr stages so it works pre- and
// post-typecheck.
func (r Resource) Has(name string) bool {
	if _, ok := r.Attrs[name]; ok {
		return true
	}
	if _, ok := r.raw[name]; ok {
		return true
	}
	_, ok := r.pending[name]
	return ok
}

// rawValue hides the language-specific value type behind a single
// coercion hook so engine.go stays free of HCL/cty.
type rawValue interface {
	Coerce(target ValueKind) (Value, error)
}

// resolvable is the language-agnostic shape of a deferred attr
// value.
type resolvable interface {
	Resolve(store []resolvedRef) (string, error)
}

type resolvedRef struct {
	Ref   Ref
	Attrs Attrs
}

// Typecheck
// -----------------------------------------------------------------------------

// typecheck applies the effective schema per resource, populating
// typed Attrs. Aggregates within phase.
func typecheck(resources []Resource) error {
	var errs []error
	for i := range resources {
		errs = append(errs, typecheckOne(&resources[i])...)
	}
	return errors.Join(errs...)
}

func typecheckOne(r *Resource) []error {
	k, err := kindFor(*r)
	if err != nil {
		return []error{err}
	}
	sch := effectiveSchema(k)
	candidates := schemaAttrNames(sch)
	var errs []error
	for _, name := range sortedKeys(r.raw) {
		pos := r.AttrSources[name]
		spec := sch.Find(name)
		if spec == nil {
			errs = append(errs, fmt.Errorf("%s%s: unknown attr %q%s",
				pos.prefix(), r.Ref(), name, hintSuffix(name, candidates)))
			continue
		}
		v, err := r.raw[name].Coerce(spec.Type)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s%s: attr %q: %w",
				pos.prefix(), r.Ref(), name, err))
			continue
		}
		r.Attrs[name] = v
	}
	r.raw = nil
	for _, name := range sortedPending(r.pending) {
		pos := r.AttrSources[name]
		spec := sch.Find(name)
		if spec == nil {
			errs = append(errs, fmt.Errorf("%s%s: unknown attr %q%s",
				pos.prefix(), r.Ref(), name, hintSuffix(name, candidates)))
			continue
		}
		if spec.Type != ValueString {
			errs = append(errs, fmt.Errorf(
				"%s%s: attr %q: refs only supported for string attrs (target is %s)",
				pos.prefix(), r.Ref(), name, spec.Type,
			))
		}
	}
	for _, spec := range sch {
		if r.Has(spec.Name) {
			continue
		}
		if spec.Required {
			errs = append(errs, fmt.Errorf("%s%s: missing required attr %q",
				r.Source.prefix(), r.Ref(), spec.Name))
			continue
		}
		r.Attrs[spec.Name] = spec.Default
	}
	return errs
}

func sortedKeys(m map[string]rawValue) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedPending(m map[string]resolvable) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Analyze
// -----------------------------------------------------------------------------

// analyze runs cross-resource structural checks. Aggregates within phase.
func analyze(resources []Resource) error {
	counts := make(map[Ref]int, len(resources))
	for _, r := range resources {
		counts[r.Ref()]++
	}
	var errs []error
	errs = append(errs, duplicateRefErrors(resources, counts)...)
	errs = append(errs, unknownDepErrors(resources, counts)...)
	return errors.Join(errs...)
}

func duplicateRefErrors(resources []Resource, counts map[Ref]int) []error {
	var errs []error
	reported := make(map[Ref]bool, len(resources))
	for _, r := range resources {
		ref := r.Ref()
		if counts[ref] > 1 && !reported[ref] {
			errs = append(errs, fmt.Errorf("%s%s: declared %d times",
				r.Source.prefix(), ref, counts[ref]))
			reported[ref] = true
		}
	}
	return errs
}

func unknownDepErrors(resources []Resource, counts map[Ref]int) []error {
	var errs []error
	for _, r := range resources {
		for _, dep := range r.deps {
			if counts[dep] != 0 {
				continue
			}
			same := make([]string, 0, len(counts))
			for ref := range counts {
				if ref.Kind == dep.Kind {
					same = append(same, ref.String())
				}
			}
			pos := r.depSources[dep]
			if pos.Path == "" {
				pos = r.Source
			}
			errs = append(errs, fmt.Errorf("%s%s: references unknown resource %q%s",
				pos.prefix(), r.Ref(), dep.String(), hintSuffix(dep.String(), same)))
		}
	}
	return errs
}

// Verify
// -----------------------------------------------------------------------------

// verify runs post-resolve semantic checks. Aggregates within phase.
func verify(resources []Resource) error {
	var errs []error
	errs = append(errs, kindValueErrors(resources)...)
	errs = append(errs, identityCollisionErrors(resources)...)
	return errors.Join(errs...)
}

func kindValueErrors(resources []Resource) []error {
	var errs []error
	for _, r := range resources {
		k, err := kindFor(r)
		if err != nil {
			continue
		}
		if err := k.Validate(r); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// Two distinct refs claiming the same live resource would flip-flop
// the inventory every tick.
func identityCollisionErrors(resources []Resource) []error {
	first := map[identityBucket]Ref{}
	var errs []error
	for _, r := range resources {
		k, err := kindFor(r)
		if err != nil {
			continue
		}
		b := identityBucketFor(r.Kind, r.Attrs, k.Identify())
		if prev, ok := first[b]; ok {
			errs = append(
				errs,
				fmt.Errorf("%s and %s declare the same identity (%s)", prev, r.Ref(), b.ident),
			)
			continue
		}
		first[b] = r.Ref()
	}
	return errs
}

// ValidatePath is the shared toolbox check Kinds use for any
// filesystem-path attr. Absolute and already-normalized; verbatim
// from there (no shell expansion, no symlink chasing).
func ValidatePath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path %q must be absolute", path)
	}
	if filepath.Clean(path) != path {
		return fmt.Errorf("path %q must be normalized (no .., //, trailing /)", path)
	}
	return nil
}

// Resolve
// -----------------------------------------------------------------------------

// resolve folds ref-bearing attrs in dependency order. Cycles,
// unknown refs, and type mismatches at eval time are snapshot-level
// faults; runtime failure of an upstream apply is not.
func resolve(resources []Resource) ([]Resource, error) {
	sorted, err := topoSortResources(resources)
	if err != nil {
		return nil, err
	}
	var store []resolvedRef
	var errs []error
	for i := range sorted {
		r := &sorted[i]
		for name, p := range r.pending {
			val, err := p.Resolve(store)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: eval %q: %w", r.Ref(), name, err))
				continue
			}
			r.Attrs[name] = StringValue(val)
		}
		store = append(store, resolvedRef{Ref: r.Ref(), Attrs: r.Attrs})
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return sorted, nil
}

// Apply
// -----------------------------------------------------------------------------

// applyAll iterates the snapshot. When bo is non-nil, failing
// resources get skipped until their backoff expires. Pass nil for
// one-shot Reconcile.
func applyAll(
	ctx context.Context,
	resources []Resource,
	inv *Inventory,
	log Log,
	bo *backoff,
) error {
	var errs []error
	now := time.Now()
	for _, r := range resources {
		if ctx.Err() != nil {
			return errors.Join(errs...)
		}
		ref := r.Ref()
		if !bo.due(ref, now) {
			log.Debug(ctx, "backoff skip", "ref", ref, "until", bo.entries[ref].nextRetry)
			continue
		}
		if err := applyOne(ctx, r, inv, log); err != nil {
			errs = append(errs, err)
			bo.failure(ref, now)
			continue
		}
		bo.success(ref)
		if err := log.Err(); err != nil {
			// Aborting on sink failure caps the audit gap at one
			// resource instead of acting without recording.
			errs = append(errs, err)
			return errors.Join(errs...)
		}
	}
	return errors.Join(errs...)
}

func applyOne(ctx context.Context, r Resource, inv *Inventory, log Log) error {
	k, err := kindFor(r)
	if err != nil {
		return err
	}
	ref := r.Ref()
	was := inv.Has(ref)

	state, err := k.Check(ctx, r)
	if err != nil {
		return err
	}

	if was && state == StateMatching {
		log.Debug(ctx, "in sync", "ref", ref)
		return nil
	}

	if !was && state != StateMissing && !r.Attrs.GetBool("adopt") {
		log.Emit(ctx, CodeApplyHalted, &ref, "state", state.String())
		return nil
	}

	if state != StateMatching {
		if err := k.Apply(ctx, r, log); err != nil {
			return err
		}
	}

	keys := k.Identify()
	sort.Strings(keys)
	ident := make(Attrs, len(keys))
	fields := make([]any, 0, 2*len(keys)+4)
	for _, key := range keys {
		v := r.Attrs[key]
		ident[key] = v
		fields = append(fields, key, v.Str)
	}
	fields = append(fields, "action", actionFor(was, state), "deps", refsToString(r.deps))
	log.Emit(ctx, CodeApplySuccess, &ref, fields...)
	inv.Add(ref, ident, r.deps)
	return nil
}

// reconcileRenames detects refs whose identity attrs match between
// the prior inventory and the new snapshot under a different name,
// and moves the inventory entry in place of churning destroy+create.
// Emits CodeApplyRenamed for each move so the action log preserves
// it across restarts.
func reconcileRenames(ctx context.Context, snap []Resource, inv *Inventory, log Log) {
	type snapHit struct {
		ref      Ref
		resource Resource
		kind     Kind
	}
	snapByIdent := map[identityBucket]snapHit{}
	for _, r := range snap {
		k, err := kindFor(r)
		if err != nil {
			continue
		}
		snapByIdent[identityBucketFor(r.Kind, r.Attrs, k.Identify())] = snapHit{
			ref: r.Ref(), resource: r, kind: k,
		}
	}
	for _, oldRef := range inv.Orphans(snap) {
		attrs, _, ok := inv.Get(oldRef)
		if !ok {
			continue
		}
		k, ok := kinds[oldRef.Kind]
		if !ok {
			continue
		}
		ib := identityBucketFor(oldRef.Kind, attrs, k.Identify())
		hit, ok := snapByIdent[ib]
		if !ok {
			continue
		}
		emitRenamed(ctx, log, oldRef, hit.ref, hit.resource, hit.kind)
		inv.Rename(oldRef, hit.ref)
	}
}

func emitRenamed(ctx context.Context, log Log, from, to Ref, r Resource, k Kind) {
	keys := k.Identify()
	sort.Strings(keys)
	fields := make([]any, 0, 2*len(keys)+4)
	fields = append(fields, "from", from.String())
	for _, key := range keys {
		fields = append(fields, key, r.Attrs[key].Str)
	}
	fields = append(fields, "deps", refsToString(r.deps))
	log.Emit(ctx, CodeApplyRenamed, &to, fields...)
}

type identityBucket struct{ kind, ident string }

func identityBucketFor(kind string, attrs Attrs, identityKeys []string) identityBucket {
	keys := append([]string{}, identityKeys...)
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key + "=" + attrs.GetString(key)
	}
	return identityBucket{kind: kind, ident: strings.Join(parts, ",")}
}

// identityDrifts returns refs present in both snap and inv whose
// identity attrs differ. The old live state under those refs needs to
// be destroyed alongside ordinary orphans so destroyOrder can route
// children-before-parents cleanup uniformly.
func identityDrifts(snap []Resource, inv *Inventory) []Ref {
	var drifts []Ref
	for _, r := range snap {
		ref := r.Ref()
		prior, _, ok := inv.Get(ref)
		if !ok {
			continue
		}
		k, err := kindFor(r)
		if err != nil {
			continue
		}
		if !sameIdentity(prior, r.Attrs, k.Identify()) {
			drifts = append(drifts, ref)
		}
	}
	return drifts
}

// sameIdentity reports whether two attr sets agree on every identity
// key. Identity attrs are string-typed by constraint, so Value equality
// is byte-for-byte.
func sameIdentity(prior, current Attrs, keys []string) bool {
	for _, k := range keys {
		if prior[k] != current[k] {
			return false
		}
	}
	return true
}

// actionFor classifies a successful apply for the operator-facing
// renderers. By the time applyOne reaches the emit, in-sync and halt
// cases are already filtered out, so the remaining states map cleanly.
func actionFor(was bool, state State) string {
	switch {
	case state == StateMissing:
		return "create"
	case !was:
		return "adopt"
	default:
		return "update"
	}
}

func refsToString(refs []Ref) string {
	if len(refs) == 0 {
		return ""
	}
	parts := make([]string, len(refs))
	for i, r := range refs {
		parts[i] = r.String()
	}
	return strings.Join(parts, ",")
}

// destroyAll walks orphans in reverse-topo order (dependents before
// deps). Shares backoff with applyAll so a flapping resource is
// paced the same whether it's failing apply or destroy.
func destroyAll(ctx context.Context, refs []Ref, inv *Inventory, log Log, bo *backoff) error {
	var errs []error
	now := time.Now()
	for _, ref := range destroyOrder(refs, inv) {
		if ctx.Err() != nil {
			return errors.Join(errs...)
		}
		if !bo.due(ref, now) {
			log.Debug(ctx, "backoff skip", "ref", ref, "until", bo.entries[ref].nextRetry)
			continue
		}
		attrs, _, ok := inv.Get(ref)
		if !ok {
			continue
		}
		if err := destroyOne(ctx, ref, attrs, log); err != nil {
			errs = append(errs, err)
			bo.failure(ref, now)
			continue
		}
		bo.success(ref)
		inv.Remove(ref)
		if err := log.Err(); err != nil {
			errs = append(errs, err)
			return errors.Join(errs...)
		}
	}
	return errors.Join(errs...)
}

func destroyOne(ctx context.Context, ref Ref, attrs Attrs, log Log) error {
	k, ok := kinds[ref.Kind]
	if !ok {
		err := fmt.Errorf("%s: unknown kind %q%s",
			ref, ref.Kind, hintSuffix(ref.Kind, kindNames()))
		log.Emit(ctx, CodeDestroyFailed, &ref, "err", err)
		return err
	}
	return k.Destroy(ctx, ref, attrs, log)
}
