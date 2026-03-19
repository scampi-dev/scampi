// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"go.starlark.net/starlark"

	"scampi.dev/scampi/spec"
)

// StarlarkPkgSource wraps a spec.PkgSourceRef as an opaque Starlark value.
type StarlarkPkgSource struct {
	Ref spec.PkgSourceRef
}

func (s *StarlarkPkgSource) String() string {
	switch s.Ref.Kind {
	case spec.PkgSourceNative:
		return "system()"
	case spec.PkgSourceApt:
		return fmt.Sprintf("apt_repo(%q)", s.Ref.URL)
	case spec.PkgSourceDnf:
		return fmt.Sprintf("dnf_repo(%q)", s.Ref.URL)
	default:
		return "pkg_source(?)"
	}
}

func (s *StarlarkPkgSource) Type() string         { return "pkg_source" }
func (s *StarlarkPkgSource) Freeze()              {}
func (s *StarlarkPkgSource) Truth() starlark.Bool { return starlark.True }

func (s *StarlarkPkgSource) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: pkg_source")
}

// system()
// -----------------------------------------------------------------------------

func builtinSystem(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs("system", args, kwargs, 0); err != nil {
		return nil, err
	}
	return &StarlarkPkgSource{
		Ref: spec.PkgSourceRef{Kind: spec.PkgSourceNative},
	}, nil
}

// apt_repo(url, key_url, components?, suite?)
// -----------------------------------------------------------------------------

func builtinAptRepo(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		rawURL     string
		keyURL     string
		components *starlark.List
		suite      string
	)
	if err := starlark.UnpackArgs("apt_repo", args, kwargs,
		"url", &rawURL,
		"key_url", &keyURL,
		"components?", &components,
		"suite?", &suite,
	); err != nil {
		return nil, err
	}

	urlSpan := resolveArgSpan(thread, "url")

	if err := validateHTTPURL(rawURL, urlSpan); err != nil {
		return nil, err
	}

	keySpan := resolveArgSpan(thread, "key_url")
	if err := validateHTTPURL(keyURL, keySpan); err != nil {
		return nil, err
	}

	var comps []string
	if components != nil {
		var err error
		comps, err = stringList(components, "apt_repo", "components")
		if err != nil {
			return nil, err
		}
	}

	return &StarlarkPkgSource{
		Ref: spec.PkgSourceRef{
			Kind:       spec.PkgSourceApt,
			Name:       repoSlug(rawURL),
			URL:        rawURL,
			KeyURL:     keyURL,
			Components: comps,
			Suite:      suite,
		},
	}, nil
}

// dnf_repo(url, key_url?)
// -----------------------------------------------------------------------------

func builtinDnfRepo(
	thread *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		rawURL string
		keyURL string
	)
	if err := starlark.UnpackArgs("dnf_repo", args, kwargs,
		"url", &rawURL,
		"key_url?", &keyURL,
	); err != nil {
		return nil, err
	}

	urlSpan := resolveArgSpan(thread, "url")
	if err := validateHTTPURL(rawURL, urlSpan); err != nil {
		return nil, err
	}

	if keyURL != "" {
		keySpan := resolveArgSpan(thread, "key_url")
		if err := validateHTTPURL(keyURL, keySpan); err != nil {
			return nil, err
		}
	}

	return &StarlarkPkgSource{
		Ref: spec.PkgSourceRef{
			Kind:   spec.PkgSourceDnf,
			Name:   repoSlug(rawURL),
			URL:    rawURL,
			KeyURL: keyURL,
		},
	}, nil
}

// Helpers
// -----------------------------------------------------------------------------

func validateHTTPURL(rawURL string, span spec.SourceSpan) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return &RemoteURLError{
			URL:    rawURL,
			Detail: fmt.Sprintf("invalid url: %v", err),
			Source: span,
		}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return &RemoteURLError{
			URL:    rawURL,
			Detail: fmt.Sprintf("url scheme must be http or https, got %q", u.Scheme),
			Source: span,
		}
	}
	return nil
}

func repoSlug(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		h := sha256.Sum256([]byte(rawURL))
		return hex.EncodeToString(h[:8])
	}
	// Use host + first path segment for a readable slug
	host := strings.ReplaceAll(u.Hostname(), ".", "-")
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		return host + "-" + parts[0]
	}
	return host
}
