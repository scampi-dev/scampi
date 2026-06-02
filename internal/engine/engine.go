// SPDX-License-Identifier: GPL-3.0-only

// Package engine reconciles desired-state snapshots against the
// real filesystem. Apply runs once; Run polls and reconciles
// continuously.
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

// Code is the stable identifier on every emission. Sinks classify by
// exact match (lifecycle, log severity, etc) and decide what to keep.
type Code string

const (
	CodeSnapshotReceived Code = "snapshot.received"
	CodeSnapshotRejected Code = "snapshot.rejected"
	CodeApplyStart       Code = "apply.start"
	CodeApplySuccess     Code = "apply.success"
	CodeApplyFailed      Code = "apply.failed"
	CodeDestroyStart     Code = "destroy.start"
	CodeDestroySuccess   Code = "destroy.success"
	CodeDestroyFailed    Code = "destroy.failed"

	CodeLogDebug Code = "log.debug"
	CodeLogInfo  Code = "log.info"
	CodeLogWarn  Code = "log.warn"
	CodeLogError Code = "log.error"
)

// IsLogCode is true for the convenience-logger codes (log.debug et al).
// Sinks like the action log skip these; only stable lifecycle events
// belong on disk.
func IsLogCode(c Code) bool {
	switch c {
	case CodeLogDebug, CodeLogInfo, CodeLogWarn, CodeLogError:
		return true
	}
	return false
}

// Emitter is the sink contract. Implementations: slog (renders to
// stderr), action (JSONL file), fanout (multiple Emitters), Discard.
// Log itself implements Emitter by delegating to a wrapped Emitter,
// so it can be passed wherever an Emitter is expected.
type Emitter interface {
	Emit(ctx context.Context, code Code, ref *Ref, args ...any)
}

// Log wraps an Emitter and adds convenience Debug/Info/Warn/Error
// helpers that funnel through Emit with the matching CodeLog* tag.
type Log struct {
	e Emitter
}

func NewLog(e Emitter) Log { return Log{e: e} }

func (l Log) Emit(ctx context.Context, code Code, ref *Ref, args ...any) {
	l.e.Emit(ctx, code, ref, args...)
}

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

// discardEmitter drops every emission.
type discardEmitter struct{}

func (discardEmitter) Emit(context.Context, Code, *Ref, ...any) {}

// Discard is a Log that drops every emission.
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

func Apply(ctx context.Context, dir string, inv *Inventory, log Log) error {
	snap, err := snapshot(ctx, dir, log)
	if err != nil {
		return err
	}
	var errs []error
	_, toDestroy := inv.Diff(snap)
	if err := destroyAll(ctx, toDestroy, inv, log, nil); err != nil {
		errs = append(errs, err)
	}
	if err := applyAll(ctx, snap, inv, log, nil); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrApplyFailed, errors.Join(errs...))
	}
	return nil
}

// Run polls dir and reconciles forever. Errors and snapshot rejects
// do not stop the loop so the operator can fix configs in place while
// reconciliation continues against the last-good snapshot. Inventory
// persists across ticks.
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
			s, serr := snapshot(ctx, dir, log)
			if serr != nil {
				logReconcileErr(ctx, log, serr)
			} else {
				snap = s
			}
			lastRev = rev
		}
		if snap != nil {
			_, toDestroy := inv.Diff(snap)
			if err := destroyAll(ctx, toDestroy, inv, log, bo); err != nil {
				logReconcileErr(ctx, log, fmt.Errorf("%w: %w", ErrApplyFailed, err))
			}
			if err := applyAll(ctx, snap, inv, log, bo); err != nil {
				logReconcileErr(ctx, log, fmt.Errorf("%w: %w", ErrApplyFailed, err))
			}
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

// snapshot parses + validates + resolves dir into apply-ready
// resources. All faults bucket as ErrSnapshotRejected.
func snapshot(ctx context.Context, dir string, log Log) ([]Resource, error) {
	resources, err := parseDir(ctx, log, dir)
	if err != nil {
		log.Emit(ctx, CodeSnapshotRejected, nil, "phase", "parse", "err", err)
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
	}
	if verr := validate(resources); verr != nil {
		log.Emit(ctx, CodeSnapshotRejected, nil, "phase", "validate", "err", verr)
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, verr)
	}
	sorted, rerr := resolve(resources)
	if rerr != nil {
		log.Emit(ctx, CodeSnapshotRejected, nil, "phase", "resolve", "err", rerr)
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, rerr)
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

// Resource is one parsed top-level block. Attrs holds literal values
// resolved at parse time; ref-bearing attrs live in pending until the
// resolve phase folds them back into Attrs.
type Resource struct {
	Kind  string
	Name  string
	Attrs map[string]string

	pending map[string]resolvable
	deps    []Ref
}

func (r Resource) Ref() Ref { return Ref{Kind: r.Kind, Name: r.Name} }

// resolvable is the language-agnostic shape of a deferred attr value.
// HCL's impl lives in hcl.go; swapping the language replaces that
// file without touching the rest of the engine.
type resolvable interface {
	Resolve(store []resolvedRef) (string, error)
}

// resolvedRef is one entry in the resolve store: a Ref plus the
// resource's already-folded attrs.
type resolvedRef struct {
	Ref   Ref
	Attrs map[string]string
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
	k, err := kindFor(r)
	if err != nil {
		return err
	}
	return k.Validate(r)
}

func hasAttr(r Resource, name string) bool {
	if _, ok := r.Attrs[name]; ok {
		return true
	}
	_, ok := r.pending[name]
	return ok
}

// Resolve
// -----------------------------------------------------------------------------

// resolve topo-sorts then folds ref-bearing attrs in dependency
// order. Cycles, unknown refs, and type mismatches at eval time are
// snapshot-level faults; runtime failure of an upstream apply is not.
func resolve(resources []Resource) ([]Resource, error) {
	sorted, err := topoSort(resources)
	if err != nil {
		return nil, err
	}
	var store []resolvedRef
	var errs []error
	for i := range sorted {
		r := &sorted[i]
		for name, p := range r.pending {
			val, perr := p.Resolve(store)
			if perr != nil {
				errs = append(errs, fmt.Errorf("%s: eval %q: %w", r.Ref(), name, perr))
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

// applyAll iterates the snapshot. When bo is non-nil, resources past
// a previous failure get skipped until their backoff expires; success
// clears the entry, failure extends it. Pass nil for one-shot Apply
// (no retry pressure to absorb).
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
	}
	return errors.Join(errs...)
}

// Backoff
// -----------------------------------------------------------------------------

// backoff tracks per-Ref retry deadlines. Methods are nil-safe so
// Apply can pass nil and the loop can pass a real one.
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

// applyOne dispatches to the Kind's Apply and handles inventory
// effects. apply.success fires when work was done OR on first-sight
// adoption (in-sync but not yet in inventory). A routine in-sync tick
// on an already-managed resource is silent.
func applyOne(ctx context.Context, r Resource, inv *Inventory, log Log) error {
	k, err := kindFor(r)
	if err != nil {
		return err
	}
	ref := r.Ref()
	was := inv.Has(ref)
	inSync, aerr := k.Apply(ctx, r, log)
	if aerr != nil {
		return aerr
	}
	if inSync && was {
		return nil
	}
	path := r.Attrs["path"]
	log.Emit(ctx, CodeApplySuccess, &ref, "path", path)
	inv.Add(ref, map[string]string{"path": path})
	return nil
}

// destroyAll walks orphan refs and destroys each via its Kind. Shares
// the same backoff state as applyAll so a flapping resource is paced
// the same way whether it's failing apply or failing destroy.
func destroyAll(ctx context.Context, refs []Ref, inv *Inventory, log Log, bo *backoff) error {
	var errs []error
	now := time.Now()
	for _, ref := range refs {
		if !bo.due(ref, now) {
			log.Debug(ctx, "backoff skip", "ref", ref, "until", bo.entries[ref].nextRetry)
			continue
		}
		attrs, ok := inv.Get(ref)
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
	}
	return errors.Join(errs...)
}

func destroyOne(ctx context.Context, ref Ref, attrs map[string]string, log Log) error {
	k, ok := kinds[ref.Kind]
	if !ok {
		err := fmt.Errorf("%s: unknown kind", ref)
		log.Emit(ctx, CodeDestroyFailed, &ref, "err", err)
		return err
	}
	return k.Destroy(ctx, ref, attrs, log)
}
