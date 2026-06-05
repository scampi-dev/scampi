// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"scampi.dev/scampi/internal/engine"
)

// jsonRenderer serializes every emission as one JSON object per
// line. Pairs well with log forwarders (logstash, fluentd) and ad-hoc
// jq queries. No event suppression: the consumer decides what to
// surface.
type jsonRenderer struct {
	mu        sync.Mutex
	enc       *json.Encoder
	verbosity Verbosity
}

func newJSONRenderer(out io.Writer, v Verbosity) *jsonRenderer {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return &jsonRenderer{enc: enc, verbosity: v}
}

func (*jsonRenderer) Err() error { return nil }

func (r *jsonRenderer) Emit(_ context.Context, code engine.Code, ref *engine.Ref, args ...any) {
	if !r.admit(code) {
		return
	}
	rec := map[string]any{
		"ts":   time.Now().Format(time.RFC3339Nano),
		"code": string(code),
	}
	if ref != nil {
		rec["ref"] = ref.String()
	}
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok {
			continue
		}
		v := args[i+1]
		if e, ok := v.(error); ok {
			v = e.Error()
		}
		rec[k] = v
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.enc.Encode(rec)
}

// admit gates log.* codes by verbosity. Lifecycle codes always pass
// since they're the meaningful event stream.
func (r *jsonRenderer) admit(code engine.Code) bool {
	switch code {
	case engine.CodeLogInfo:
		return r.verbosity >= VerbosityDefault
	case engine.CodeLogDebug:
		return r.verbosity >= VerbosityVerbose
	}
	return true
}
