// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"scampi.dev/scampi/internal/platform"
)

// ActionLog writes lifecycle events as JSONL to a segmented directory.
// log.* codes get filtered so the action log stays the stable
// machine-readable stream. The dir is held under an exclusive
// process-level lock so two scampis pointed at the same dir can't
// interleave writes.
//
// failed is sticky: the first Encode or Sync error captures here and
// every subsequent Emit short-circuits. The engine polls Err() so it
// can abort the reconcile pass on first failure instead of acting
// without recording.
type ActionLog struct {
	mu     sync.Mutex
	f      *os.File
	enc    *json.Encoder
	lock   platform.Lock
	failed error
}

func NewActionLog(dir string, locker platform.Locker) (*ActionLog, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("action log dir: %w", err)
	}
	lock, err := locker.Acquire(filepath.Join(dir, ".lock"))
	if err != nil {
		return nil, fmt.Errorf("action log lock: %w", err)
	}
	path, err := activeSegment(dir)
	if err != nil {
		_ = lock.Release()
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_ = lock.Release()
		return nil, err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return &ActionLog{f: f, enc: enc, lock: lock}, nil
}

func (a *ActionLog) Close() error {
	ferr := a.f.Close()
	lerr := a.lock.Release()
	if ferr != nil {
		return ferr
	}
	return lerr
}

func (a *ActionLog) Err() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.failed
}

func (a *ActionLog) Emit(_ context.Context, code Code, ref *Ref, args ...any) {
	if IsLogCode(code) {
		return
	}
	rec := map[string]any{"ts": time.Now(), "code": string(code)}
	if ref != nil {
		rec["ref"] = ref.String()
	}
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok {
			continue
		}
		rec[k] = args[i+1]
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.failed != nil {
		return
	}
	// fsync after each event so a crash mid-tick leaves no buffered
	// events stranded; replay sees exactly what disk has.
	if err := a.enc.Encode(rec); err != nil {
		a.failed = fmt.Errorf("action log encode: %w", err)
		return
	}
	if err := a.f.Sync(); err != nil {
		a.failed = fmt.Errorf("action log fsync: %w", err)
		return
	}
}

// activeSegment returns the highest-numbered *.jsonl segment in dir,
// or 0001.jsonl when the dir holds none. 4-digit zero padding makes
// lexical sort match numeric sort up to 9999 segments.
func activeSegment(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return filepath.Join(dir, "0001.jsonl"), nil
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

// LoadInventory replays *.jsonl segments under dir in lexical order
// and folds the events into a fresh Inventory. A missing or empty
// dir yields an empty inventory.
func LoadInventory(dir string) (*Inventory, error) {
	inv := NewInventory()
	segments, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(segments)
	for _, seg := range segments {
		if err := foldSegment(seg, inv); err != nil {
			return nil, fmt.Errorf("%s: %w", seg, err)
		}
	}
	return inv, nil
}

func foldSegment(path string, inv *Inventory) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(f)
	for {
		var raw map[string]any
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		code, _ := raw["code"].(string)
		refStr, _ := raw["ref"].(string)
		delete(raw, "code")
		delete(raw, "ts")
		delete(raw, "ref")
		attrs := make(map[string]string, len(raw))
		for k, v := range raw {
			if s, ok := v.(string); ok {
				attrs[k] = s
			}
		}
		inv.Fold(Code(code), parseRef(refStr), attrs)
	}
}

func parseRef(s string) Ref {
	kind, name, ok := strings.Cut(s, ".")
	if !ok {
		return Ref{Kind: s}
	}
	return Ref{Kind: kind, Name: name}
}
