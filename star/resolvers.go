// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"go.starlark.net/starlark"

	"scampi.dev/scampi/spec"
)

// StarlarkSource wraps a spec.SourceRef as an opaque Starlark value.
type StarlarkSource struct {
	Ref spec.SourceRef
}

func (s *StarlarkSource) String() string {
	switch s.Ref.Kind {
	case spec.SourceLocal:
		return fmt.Sprintf("local(%q)", s.Ref.Path)
	case spec.SourceInline:
		return fmt.Sprintf("inline(%q)", s.Ref.Content)
	default:
		return "source(?)"
	}
}

func (s *StarlarkSource) Type() string         { return "source" }
func (s *StarlarkSource) Freeze()              {}
func (s *StarlarkSource) Truth() starlark.Bool { return starlark.True }

func (s *StarlarkSource) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: source")
}

// local(path)
// -----------------------------------------------------------------------------

func builtinLocal(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs("local", args, kwargs, 1, &path); err != nil {
		return nil, err
	}
	return &StarlarkSource{
		Ref: spec.SourceRef{
			Kind: spec.SourceLocal,
			Path: path,
		},
	}, nil
}

// inline(content)
// -----------------------------------------------------------------------------

func builtinInline(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var content string
	if err := starlark.UnpackPositionalArgs("inline", args, kwargs, 1, &content); err != nil {
		return nil, err
	}

	c := threadCollector(thread)

	h := sha256.Sum256([]byte(content))
	hexHash := hex.EncodeToString(h[:])
	cachePath := ".scampi-cache/inline/" + hexHash

	ctx := context.Background()
	if err := c.src.EnsureDir(ctx, ".scampi-cache/inline"); err != nil {
		return nil, fmt.Errorf("inline: creating cache dir: %w", err)
	}
	if err := c.src.WriteFile(ctx, cachePath, []byte(content)); err != nil {
		return nil, fmt.Errorf("inline: writing cache file: %w", err)
	}

	return &StarlarkSource{
		Ref: spec.SourceRef{
			Kind:    spec.SourceInline,
			Path:    cachePath,
			Content: content,
		},
	}, nil
}
