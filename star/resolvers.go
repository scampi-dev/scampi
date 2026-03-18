// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"

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
	case spec.SourceRemote:
		return fmt.Sprintf("remote(%q)", s.Ref.URL)
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

// remote(url, checksum?)
// -----------------------------------------------------------------------------

func builtinRemote(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var rawURL, checksum string
	if err := starlark.UnpackArgs("remote", args, kwargs,
		"url", &rawURL,
		"checksum?", &checksum,
	); err != nil {
		return nil, err
	}

	urlSpan := resolveArgSpan(thread, "url")

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, &RemoteURLError{
			URL:    rawURL,
			Detail: fmt.Sprintf("invalid url: %v", err),
			Source: urlSpan,
		}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, &RemoteURLError{
			URL:    rawURL,
			Detail: fmt.Sprintf("url scheme must be http or https, got %q", u.Scheme),
			Source: urlSpan,
		}
	}

	if checksum != "" {
		if detail := validateChecksum(checksum); detail != "" {
			return nil, &RemoteChecksumError{
				Checksum: checksum,
				Detail:   detail,
				Source:   resolveArgSpan(thread, "checksum"),
			}
		}
	}

	h := sha256.Sum256([]byte(rawURL))
	dirHash := hex.EncodeToString(h[:16])
	filename := path.Base(u.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}
	cachePath := ".scampi-cache/remote/" + dirHash + "/" + filename

	return &StarlarkSource{
		Ref: spec.SourceRef{
			Kind:     spec.SourceRemote,
			Path:     cachePath,
			URL:      rawURL,
			Checksum: checksum,
		},
	}, nil
}

func validateChecksum(s string) string {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return fmt.Sprintf("checksum must be \"algo:hex\", got %q", s)
	}
	algo := parts[0]
	switch algo {
	case "sha256", "sha384", "sha512", "sha1", "md5":
		return ""
	default:
		return fmt.Sprintf("unsupported checksum algorithm %q", algo)
	}
}

// resolveArgSpan returns the source span for a kwarg value in a remote() call,
// falling back to the call site if the AST walk fails.
func resolveArgSpan(thread *starlark.Thread, name string) spec.SourceSpan {
	pos := callerPosition(thread)
	call := findCallFromThread(thread, pos)
	if call != nil {
		if vs, ok := kwargValueSpan(call, name); ok {
			return vs
		}
	}
	return posToSpan(pos)
}
