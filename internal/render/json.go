// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"scampi.dev/scampi/internal/engine"
)

type JSONRenderer struct {
	mu        sync.Mutex
	enc       *json.Encoder
	verbosity Verbosity
}

func NewJSONRenderer(out io.Writer, v Verbosity) *JSONRenderer {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return &JSONRenderer{enc: enc, verbosity: v}
}

func (*JSONRenderer) Err() error { return nil }

func (r *JSONRenderer) Emit(_ context.Context, code engine.Code, ref *engine.Ref, args ...any) {
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

// admit gates log.* by verbosity; lifecycle always passes.
func (r *JSONRenderer) admit(code engine.Code) bool {
	switch code {
	case engine.CodeLogInfo:
		return r.verbosity >= VerbosityDefault
	case engine.CodeLogDebug:
		return r.verbosity >= VerbosityVerbose
	}
	return true
}
