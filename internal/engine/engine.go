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

// Log
// -----------------------------------------------------------------------------

// Code is the stable identifier on every emission. Sinks classify
// by exact match.
type Code string

const (
	CodeSnapshotReceived Code = "snapshot.received"
	CodeSnapshotRejected Code = "snapshot.rejected"
	CodeApplyStart       Code = "apply.start"
	CodeApplySuccess     Code = "apply.success"
	CodeApplyFailed      Code = "apply.failed"
	CodeApplyHalted      Code = "apply.halted"
	CodeDestroyStart     Code = "destroy.start"
	CodeDestroySuccess   Code = "destroy.success"
	CodeDestroyFailed    Code = "destroy.failed"

	CodeLogDebug Code = "log.debug"
	CodeLogInfo  Code = "log.info"
	CodeLogWarn  Code = "log.warn"
	CodeLogError Code = "log.error"
)

// IsLifecycle reports whether c is a structural lifecycle event
// rather than a convenience log emission.
func (c Code) IsLifecycle() bool {
	switch c {
	case CodeLogDebug, CodeLogInfo, CodeLogWarn, CodeLogError:
		return false
	}
	return true
}

// Emitter is the sink contract. Err is sticky: once a sink fails it
// stays failed, so the reconcile loop can abort the pass on first
// failure instead of acting without recording.
type Emitter interface {
	Emit(ctx context.Context, code Code, ref *Ref, args ...any)
	Err() error
}

type Log struct {
	e Emitter
}

func NewLog(e Emitter) Log { return Log{e: e} }

func (l Log) Emit(ctx context.Context, code Code, ref *Ref, args ...any) {
	l.e.Emit(ctx, code, ref, args...)
}

func (l Log) Err() error { return l.e.Err() }

func (l Log) Debug(ctx context.Context, msg string, args ...any) {
	l.emitLog(ctx, CodeLogDebug, msg, args)
}

func (l Log) Info(ctx context.Context, msg string, args ...any) {
	l.emitLog(ctx, CodeLogInfo, msg, args)
}

func (l Log) Warn(ctx context.Context, msg string, args ...any) {
	l.emitLog(ctx, CodeLogWarn, msg, args)
}

func (l Log) Error(ctx context.Context, msg string, args ...any) {
	l.emitLog(ctx, CodeLogError, msg, args)
}

func (l Log) emitLog(ctx context.Context, code Code, msg string, args []any) {
	full := make([]any, 0, len(args)+2)
	full = append(full, "msg", msg)
	full = append(full, args...)
	l.Emit(ctx, code, nil, full...)
}

type discardEmitter struct{}

func (discardEmitter) Emit(context.Context, Code, *Ref, ...any) {}
func (discardEmitter) Err() error                               { return nil }

var Discard = NewLog(discardEmitter{})

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

// Plan inspects what an Apply against dir would do without touching
// live state. Reads inv; never mutates it or writes lifecycle events.
type Plan struct {
	Create  []Ref // would write a new resource (state=missing)
	Update  []Ref // would rewrite (state=diverging: drift or take-over)
	Adopt   []Ref // would claim matching live state (first time + adopt)
	Halt    []Ref // would refuse: live exists but adopt=false
	Destroy []Ref // would remove orphans
	InSync  []Ref // owned and matching - no action
}

func MakePlan(ctx context.Context, dir string, inv *Inventory, log Log) (*Plan, error) {
	snap, err := snapshot(ctx, dir, log)
	if err != nil {
		return nil, err
	}
	p := &Plan{}
	for _, r := range snap {
		ref := r.Ref()
		was := inv.Has(ref)
		k, err := kindFor(r)
		if err != nil {
			continue
		}
		state, err := k.Check(ctx, r)
		if err != nil {
			return nil, err
		}
		switch {
		case was && state == StateMatching:
			p.InSync = append(p.InSync, ref)
		case !was && state != StateMissing && !r.Adopt:
			p.Halt = append(p.Halt, ref)
		case state == StateMissing:
			p.Create = append(p.Create, ref)
		case state == StateMatching:
			p.Adopt = append(p.Adopt, ref)
		default:
			p.Update = append(p.Update, ref)
		}
	}
	p.Destroy = inv.Orphans(snap)
	return p, nil
}

func Apply(ctx context.Context, dir string, inv *Inventory, log Log) error {
	snap, err := snapshot(ctx, dir, log)
	if err != nil {
		return err
	}
	var errs []error
	orphans := inv.Orphans(snap)
	if err := destroyAll(ctx, orphans, inv, log, nil); err != nil {
		errs = append(errs, err)
	}
	if err := applyAll(ctx, snap, inv, log, nil); err != nil {
		errs = append(errs, err)
	}
	if err := log.Err(); err != nil {
		errs = append(errs, fmt.Errorf("action log: %w", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrApplyFailed, errors.Join(errs...))
	}
	return nil
}

// Run polls dir and reconciles forever. Snapshot rejects do not
// stop the loop so the operator can fix configs in place while
// reconciliation continues against the last-good snapshot.
func Run(ctx context.Context, dir string, interval time.Duration, inv *Inventory, log Log) error {
	log.Info(ctx, "starting run loop", "dir", dir, "interval", interval)
	var (
		lastRev string
		snap    []Resource
	)
	bo := newBackoff()
	for {
		rev, hashErr := hashDir(dir)
		switch {
		case hashErr != nil:
			log.Error(ctx, "hash dir", "err", hashErr)
		case rev != lastRev:
			log.Debug(ctx, "snapshot change", "rev", rev)
			s, err := snapshot(ctx, dir, log)
			if err != nil {
				logReconcileErr(ctx, log, err)
			} else {
				snap = s
			}
			lastRev = rev
		}
		if snap != nil {
			orphans := inv.Orphans(snap)
			if err := destroyAll(ctx, orphans, inv, log, bo); err != nil {
				logReconcileErr(ctx, log, fmt.Errorf("%w: %w", ErrApplyFailed, err))
			}
			if err := applyAll(ctx, snap, inv, log, bo); err != nil {
				logReconcileErr(ctx, log, fmt.Errorf("%w: %w", ErrApplyFailed, err))
			}
		}
		// Action log failure is fatal: persistence is broken, so we
		// stop reconciling rather than acting blind.
		if err := log.Err(); err != nil {
			return fmt.Errorf("action log: %w", err)
		}
		select {
		case <-ctx.Done():
			log.Info(ctx, "shutting down")
			return nil
		case <-time.After(interval):
		}
	}
}

// Pipeline
// -----------------------------------------------------------------------------

// snapshot parses, validates, and resolves dir. All faults bucket
// as ErrSnapshotRejected.
func snapshot(ctx context.Context, dir string, log Log) ([]Resource, error) {
	resources, err := parseDir(ctx, log, dir)
	if err != nil {
		log.Emit(ctx, CodeSnapshotRejected, nil, "phase", "parse", "err", err)
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
	}
	if err := validate(resources); err != nil {
		log.Emit(ctx, CodeSnapshotRejected, nil, "phase", "validate", "err", err)
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
	}
	sorted, err := resolve(resources)
	if err != nil {
		log.Emit(ctx, CodeSnapshotRejected, nil, "phase", "resolve", "err", err)
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
	}
	if err := validateKinds(sorted); err != nil {
		log.Emit(ctx, CodeSnapshotRejected, nil, "phase", "validate", "err", err)
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
	}
	if err := validateIdentities(sorted); err != nil {
		log.Emit(ctx, CodeSnapshotRejected, nil, "phase", "validate", "err", err)
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
	}
	log.Emit(ctx, CodeSnapshotReceived, nil, "resources", len(sorted))
	return sorted, nil
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

// Attrs holds attribute name -> value pairs. The same shape carries
// the full declared set on a Resource and the identity-projected
// subset stored in the inventory; call-site variable name picks
// which flavor.
type Attrs map[string]string

// Resource is one parsed block. Attrs holds literal values resolved
// at parse time; ref-bearing attrs live in pending until resolve
// folds them back into Attrs.
type Resource struct {
	Kind  string
	Name  string
	Attrs Attrs
	Adopt bool

	pending map[string]resolvable
	deps    []Ref
}

func (r Resource) Ref() Ref { return Ref{Kind: r.Kind, Name: r.Name} }

// Has reports whether the resource declared name. Validate runs
// before resolve, so pending counts too.
func (r Resource) Has(name string) bool {
	if _, ok := r.Attrs[name]; ok {
		return true
	}
	_, ok := r.pending[name]
	return ok
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

// Validate
// -----------------------------------------------------------------------------

// validate aggregates so the operator sees every fault at once.
// Pre-resolve phase: cross-resource structural checks only.
func validate(resources []Resource) error {
	var errs []error
	counts := make(map[Ref]int, len(resources))
	for _, r := range resources {
		counts[r.Ref()]++
	}
	reported := make(map[Ref]bool, len(resources))
	for _, r := range resources {
		ref := r.Ref()
		if counts[ref] > 1 && !reported[ref] {
			errs = append(errs, fmt.Errorf("%s: declared %d times", ref, counts[ref]))
			reported[ref] = true
		}
		if _, err := kindFor(r); err != nil {
			errs = append(errs, err)
		}
		for _, dep := range r.deps {
			if counts[dep] == 0 {
				errs = append(errs, fmt.Errorf("%s: references unknown resource %q", ref, dep))
			}
		}
	}
	return errors.Join(errs...)
}

// validateKinds runs after resolve so each Kind.Validate sees the
// fully-folded Attrs.
func validateKinds(resources []Resource) error {
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
	return errors.Join(errs...)
}

// validateIdentities catches two distinct refs that would claim the
// same live resource. Without this, the inventory would flip-flop
// between them every tick.
func validateIdentities(resources []Resource) error {
	type bucket struct{ kind, ident string }
	first := map[bucket]Ref{}
	var errs []error
	for _, r := range resources {
		k, err := kindFor(r)
		if err != nil {
			continue
		}
		keys := k.Identify()
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, key := range keys {
			parts[i] = key + "=" + r.Attrs[key]
		}
		ident := strings.Join(parts, ",")
		b := bucket{kind: r.Kind, ident: ident}
		if prev, ok := first[b]; ok {
			errs = append(errs, fmt.Errorf("%s and %s declare the same identity (%s)", prev, r.Ref(), ident))
			continue
		}
		first[b] = r.Ref()
	}
	return errors.Join(errs...)
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
			r.Attrs[name] = val
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
// one-shot Apply.
func applyAll(ctx context.Context, resources []Resource, inv *Inventory, log Log, bo *backoff) error {
	var errs []error
	now := time.Now()
	for _, r := range resources {
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

// Backoff
// -----------------------------------------------------------------------------

// backoff tracks per-Ref retry deadlines. Methods are nil-safe.
type backoff struct {
	entries map[Ref]*backoffEntry
}

type backoffEntry struct {
	nextRetry time.Time
	attempts  int
}

func newBackoff() *backoff { return &backoff{entries: map[Ref]*backoffEntry{}} }

func (b *backoff) due(ref Ref, now time.Time) bool {
	if b == nil {
		return true
	}
	e, ok := b.entries[ref]
	if !ok {
		return true
	}
	return !now.Before(e.nextRetry)
}

func (b *backoff) success(ref Ref) {
	if b == nil {
		return
	}
	delete(b.entries, ref)
}

func (b *backoff) failure(ref Ref, now time.Time) {
	if b == nil {
		return
	}
	e, ok := b.entries[ref]
	if !ok {
		e = &backoffEntry{}
		b.entries[ref] = e
	}
	e.attempts++
	e.nextRetry = now.Add(backoffDelay(e.attempts))
}

// backoffDelay doubles per attempt starting at 1s, capped at 5 min.
func backoffDelay(attempts int) time.Duration {
	if attempts < 1 {
		return 0
	}
	shift := min(attempts-1, 30)
	d := time.Second << shift
	const maxDelay = 5 * time.Minute
	if d > maxDelay || d < 0 {
		return maxDelay
	}
	return d
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

	if !was && state != StateMissing && !r.Adopt {
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
	fields := make([]any, 0, 2*len(keys)+2)
	for _, key := range keys {
		v := r.Attrs[key]
		ident[key] = v
		fields = append(fields, key, v)
	}
	fields = append(fields, "deps", refsToString(r.deps))
	log.Emit(ctx, CodeApplySuccess, &ref, fields...)
	inv.Add(ref, ident, r.deps)
	return nil
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
		err := fmt.Errorf("%s: unknown kind", ref)
		log.Emit(ctx, CodeDestroyFailed, &ref, "err", err)
		return err
	}
	return k.Destroy(ctx, ref, attrs, log)
}
