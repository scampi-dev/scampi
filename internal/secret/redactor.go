// SPDX-License-Identifier: GPL-3.0-only

package secret

import (
	"context"
	"strings"
	"sync"
)

// ctxKey is unexported so callers can't accidentally collide with our
// context attachment. Use WithRedactor / FromContext to attach/retrieve.
type ctxKey struct{}

// WithRedactor returns a child context carrying r. Both eval (during
// link) and render (during emit) pull the redactor off the context so
// no engine signatures have to grow a parameter just to thread it.
func WithRedactor(ctx context.Context, r *Redactor) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

// FromContext returns the redactor attached to ctx, or nil. A nil
// redactor is a safe no-op for both Add and Redact, so callers don't
// need to nil-check.
func FromContext(ctx context.Context) *Redactor {
	r, _ := ctx.Value(ctxKey{}).(*Redactor)
	return r
}

// DefaultMask is the placeholder substituted for secret values in
// rendered output. Six asterisks is short enough not to disrupt line
// layout but distinct enough to grep for in transcripts.
const DefaultMask = "***SECRET***"

// minRedactLen guards against false-positive substring matches on
// trivially-short secret values. Three-character "secrets" would
// shred legitimate output. Users with secrets shorter than four
// characters have a worse problem than redaction not engaging.
const minRedactLen = 4

// Redactor accumulates secret values seen during evaluation and
// substitutes them with a mask in rendered output. Substring
// redaction is intentional: a secret consumed via string concat or
// `${...}` interpolation ends up as plain bytes in the op config
// (op fields are Go strings; the eval-side taint is lost), so
// post-hoc substring matching is the only place we can catch every
// downstream rendering — diagnostics, hints, inspect dumps, plan
// previews — uniformly.
//
// False positives are possible if a secret happens to be a common
// substring (a four-character hex prefix that appears elsewhere).
// In practice this is rare for typical passwords / API keys / tokens.
// False negatives happen only when the secret has been transformed
// (hashed, base64'd, etc.) before rendering — at which point the
// rendered value isn't really the secret.
//
// The zero value and a nil pointer are both usable as a no-op
// redactor; callers without secrets wired in get pass-through
// behavior.
type Redactor struct {
	mu      sync.RWMutex
	mask    string
	secrets map[string]struct{}
}

func NewRedactor() *Redactor {
	return &Redactor{
		mask:    DefaultMask,
		secrets: make(map[string]struct{}),
	}
}

// Add registers value as a secret to redact in subsequent output.
// Values shorter than minRedactLen are ignored (false-positive risk).
// Empty strings are ignored. Duplicates are deduplicated.
func (r *Redactor) Add(value string) {
	if r == nil || len(value) < minRedactLen {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.secrets == nil {
		r.secrets = make(map[string]struct{})
	}
	r.secrets[value] = struct{}{}
}

// Redact returns s with every known secret value replaced by the
// mask. The mask defaults to DefaultMask but may be customized via
// SetMask (e.g. for tests or grep-friendly variants).
//
// A nil receiver returns s unchanged.
func (r *Redactor) Redact(s string) string {
	if r == nil || s == "" {
		return s
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for sec := range r.secrets {
		s = strings.ReplaceAll(s, sec, r.mask)
	}
	return s
}

// Size returns the number of registered secrets — useful for tests
// and for plan-time diagnostics ("redacted N secret values").
func (r *Redactor) Size() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.secrets)
}

// SetMask overrides the default mask. Future use: project-level
// configuration so users can pick a grep-friendly token like
// `<scampi-secret>`.
func (r *Redactor) SetMask(mask string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mask = mask
}
