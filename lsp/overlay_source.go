// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"

	"scampi.dev/scampi/source"
)

// overlaySource implements source.Source by returning in-memory
// editor content for the active file path and delegating to a base
// LocalPosixSource for everything else (secrets files, templates,
// etc. that the analyzed config references).
//
// The overlay exists so the LSP can run linker.Analyze on the
// editor's current buffer state without writing it to disk first.
// All write operations are no-ops because Analyze never mutates the
// filesystem.
type overlaySource struct {
	base    source.LocalPosixSource
	path    string
	content []byte
}

func newOverlaySource(path string, content []byte) *overlaySource {
	return &overlaySource{
		path:    path,
		content: content,
	}
}

func (o *overlaySource) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if path == o.path {
		return o.content, nil
	}
	return o.base.ReadFile(ctx, path)
}

func (o *overlaySource) WriteFile(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (o *overlaySource) EnsureDir(_ context.Context, _ string) error {
	return nil
}

func (o *overlaySource) Stat(ctx context.Context, path string) (source.FileMeta, error) {
	return o.base.Stat(ctx, path)
}

// LookupEnv overrides the base behaviour: at edit time the editor's
// process environment doesn't have the apply-time env populated, and
// scampi's `std.env(NAME)` would fire `EnvVarNotSet` on every
// reference — turning every env-driven config into a sea of red
// the moment it's opened.
//
// For unset keys we return a non-empty placeholder so the eval
// treats the var as set: silences `EnvVarNotSet` AND `@std.nonempty`
// cascades on env-derived strings. parse_int's LSP-mode relaxation
// (`lang/eval.WithLSPMode`) absorbs the numeric cascade. The price
// is a slight semantic quirk in LSP eval (e.g. `std.env("X")` is
// always non-empty / always equal to the placeholder) which is
// acceptable because the LSP is for editing assistance, not a full
// apply simulation. See #264.
func (o *overlaySource) LookupEnv(key string) (string, bool) {
	if v, ok := o.base.LookupEnv(key); ok {
		return v, true
	}
	return "lsp-placeholder", true
}
