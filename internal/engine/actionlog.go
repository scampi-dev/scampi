// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"bytes"
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
)

// ActionLog writes lifecycle events as JSONL to a segmented dir.
// log.* codes get filtered out so the on-disk stream stays the
// stable machine-readable record.
//
// failed is sticky: the first Encode or Sync error captures here
// and every subsequent Emit short-circuits.
type ActionLog struct {
	mu     sync.Mutex
	f      *os.File
	enc    *json.Encoder
	failed error
}

func NewActionLog(dir string) (*ActionLog, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("action log dir: %w", err)
	}
	path, err := activeSegment(dir)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return &ActionLog{f: f, enc: enc}, nil
}

func (a *ActionLog) Close() error { return a.f.Close() }

func (a *ActionLog) Err() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.failed
}

func (a *ActionLog) Emit(_ context.Context, code Code, ref *Ref, args ...any) {
	if !code.IsLifecycle() {
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
	// fsync each event so a mid-tick crash leaves nothing buffered;
	// replay sees exactly what disk has.
	if err := a.enc.Encode(rec); err != nil {
		a.failed = fmt.Errorf("action log encode: %w", err)
		return
	}
	if err := a.f.Sync(); err != nil {
		a.failed = fmt.Errorf("action log fsync: %w", err)
		return
	}
}

// activeSegment returns the highest-numbered *.jsonl segment in dir.
// 4-digit zero padding makes lexical sort match numeric sort up to
// 9999 segments.
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

// LoadInventory folds the JSONL segments under dir into a fresh
// inventory. Strict: every line must decode cleanly. Missing or
// empty dir yields an empty inventory.
func LoadInventory(dir string) (*Inventory, error) {
	return loadInventory(dir, false)
}

// LoadInventoryLenient is the read-only counterpart for callers
// observing an action log that another process may be appending to.
// A single partial trailing line is tolerated; mid-stream corruption
// still errors.
func LoadInventoryLenient(dir string) (*Inventory, error) {
	return loadInventory(dir, true)
}

func loadInventory(dir string, lenient bool) (*Inventory, error) {
	inv := NewInventory()
	segments, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(segments)
	for i, seg := range segments {
		// Only the active (last) segment can have a partial trailing
		// line under a concurrent writer.
		segLenient := lenient && i == len(segments)-1
		if err := foldSegment(seg, inv, segLenient); err != nil {
			return nil, fmt.Errorf("%s: %w", seg, err)
		}
	}
	return inv, nil
}

func foldSegment(path string, inv *Inventory, lenient bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var complete, trailing []byte
	if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
		complete = data[:i+1]
		trailing = data[i+1:]
	} else {
		trailing = data
	}
	dec := json.NewDecoder(bytes.NewReader(complete))
	for {
		var raw map[string]any
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		code, _ := raw["code"].(string)
		refStr, _ := raw["ref"].(string)
		delete(raw, "code")
		delete(raw, "ts")
		delete(raw, "ref")
		attrs := make(Attrs, len(raw))
		for k, v := range raw {
			if s, ok := v.(string); ok {
				attrs[k] = s
			}
		}
		inv.Fold(Code(code), parseRef(refStr), attrs)
	}
	if len(trailing) > 0 && !lenient {
		return fmt.Errorf("trailing partial line: %q", trailing)
	}
	return nil
}

func parseRef(s string) Ref {
	kind, name, ok := strings.Cut(s, ".")
	if !ok {
		return Ref{Kind: s}
	}
	return Ref{Kind: kind, Name: name}
}
