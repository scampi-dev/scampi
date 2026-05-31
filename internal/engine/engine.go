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
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	sorted, err := snapshot(ctx, dir, log)
	if err != nil {
		return err
	}
	if aerr := applyAll(ctx, sorted, log); aerr != nil {
		return fmt.Errorf("%w: %w", ErrApplyFailed, aerr)
	}
	return nil
}

// snapshot parses + validates + resolves dir into apply-ready
// resources. All faults bucket as ErrSnapshotRejected.
func snapshot(ctx context.Context, dir string, log Log) ([]Resource, error) {
	resources, err := parseDir(ctx, log, dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
	}
	if verr := validate(resources); verr != nil {
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, verr)
	}
	sorted, rerr := resolve(resources)
	if rerr != nil {
		return nil, fmt.Errorf("%w: %w", ErrSnapshotRejected, rerr)
	}
	return sorted, nil
}

// runLoop re-parses only when the config hash changes but applies on
// every tick so drift in observed state gets converged. When a new
// snapshot is rejected the previous one stays active -- the design
// doc's "REJECT this snapshot, keep last-good" semantics.
func runLoop(ctx context.Context, dir string, interval time.Duration, log Log) error {
	log.Info(ctx, "starting run loop", "dir", dir, "interval", interval)
	var (
		lastRev    string
		sortedSnap []Resource
	)
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
				sortedSnap = s
			}
			lastRev = rev
		}
		if sortedSnap != nil {
			if aerr := applyAll(ctx, sortedSnap, log); aerr != nil {
				logReconcileErr(ctx, log, fmt.Errorf("%w: %w", ErrApplyFailed, aerr))
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
