// SPDX-License-Identifier: GPL-3.0-only

package sharedops

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"path"
	"time"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
)

const downloadID = "step.download"

// DownloadOp fetches a remote URL to the source cache. It runs source-side
// only — no target capabilities required.
type DownloadOp struct {
	BaseOp
	URL       string
	Checksum  spec.Checksum
	CachePath string // where to write the downloaded file
	Client    *http.Client
}

func (op *DownloadOp) metaPath() string {
	return op.CachePath + ".meta"
}

func (*DownloadOp) Timeout() time.Duration { return 10 * time.Minute }

// downloadMeta stores HTTP caching headers alongside downloaded files.
type downloadMeta struct {
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	Checksum     string `json:"checksum,omitempty"`
}

// Check
// -----------------------------------------------------------------------------

func (op *DownloadOp) Check(
	ctx context.Context,
	src source.Source,
	_ target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	cached, _ := src.Stat(ctx, op.CachePath)

	// If we have a checksum, we can verify the cached file without a network call.
	if !op.Checksum.IsZero() && cached.Exists {
		data, err := src.ReadFile(ctx, op.CachePath)
		if err == nil {
			if verifyChecksum(data, op.Checksum) {
				return spec.CheckSatisfied, nil, nil
			}
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "checksum",
				Desired: op.Checksum.String(),
			}}, nil
		}
	}

	// No checksum or no cached file — use HTTP conditional request.
	meta := op.loadMeta(ctx, src)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, op.URL, nil)
	if err != nil {
		return spec.CheckUnsatisfied, nil, DownloadError{
			URL: op.URL, Detail: fmt.Sprintf("creating request: %v", err),
			Source: op.SrcSpan,
		}
	}
	if meta.ETag != "" {
		req.Header.Set("If-None-Match", meta.ETag)
	}
	if meta.LastModified != "" {
		req.Header.Set("If-Modified-Since", meta.LastModified)
	}

	resp, err := op.client().Do(req)
	if err != nil {
		// Network error during check — report unsatisfied, not an error.
		// The download will fail during apply with a proper error.
		if !cached.Exists {
			return spec.CheckUnsatisfied, []spec.DriftDetail{{
				Field:   "remote",
				Desired: op.URL,
			}}, nil
		}
		// Cached file exists but we can't verify — assume stale.
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "remote",
			Current: "(cached, unverified)",
			Desired: op.URL,
		}}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified && cached.Exists {
		return spec.CheckSatisfied, nil, nil
	}

	if !cached.Exists {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "remote",
			Desired: op.URL,
		}}, nil
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "remote",
		Current: "(stale)",
		Desired: op.URL,
	}}, nil
}

// Execute
// -----------------------------------------------------------------------------

func (op *DownloadOp) Execute(
	ctx context.Context,
	src source.Source,
	_ target.Target,
) (spec.Result, error) {
	// Idempotency re-check for checksum-verified files.
	if !op.Checksum.IsZero() {
		data, err := src.ReadFile(ctx, op.CachePath)
		if err == nil && verifyChecksum(data, op.Checksum) {
			return spec.Result{Changed: false}, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, op.URL, nil)
	if err != nil {
		return spec.Result{}, DownloadError{
			URL: op.URL, Detail: fmt.Sprintf("creating request: %v", err),
			Source: op.SrcSpan,
		}
	}

	resp, err := op.client().Do(req)
	if err != nil {
		return spec.Result{}, DownloadError{
			URL: op.URL, Detail: fmt.Sprintf("request failed: %v", err),
			Source: op.SrcSpan,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return spec.Result{}, DownloadError{
			URL:    op.URL,
			Detail: fmt.Sprintf("HTTP %d %s", resp.StatusCode, resp.Status),
			Source: op.SrcSpan,
		}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return spec.Result{}, DownloadError{
			URL: op.URL, Detail: fmt.Sprintf("reading response: %v", err),
			Source: op.SrcSpan,
		}
	}

	if !op.Checksum.IsZero() {
		if !verifyChecksum(data, op.Checksum) {
			return spec.Result{}, ChecksumMismatchError{
				URL:      op.URL,
				Expected: op.Checksum.String(),
				Got:      computeChecksum(data, op.Checksum),
				Source:   op.SrcSpan,
			}
		}
	}

	cacheDir := path.Dir(op.CachePath)
	if err := src.EnsureDir(ctx, cacheDir); err != nil {
		return spec.Result{}, DownloadError{
			URL: op.URL, Detail: fmt.Sprintf("creating cache dir: %v", err),
			Source: op.SrcSpan,
		}
	}

	if err := src.WriteFile(ctx, op.CachePath, data); err != nil {
		return spec.Result{}, DownloadError{
			URL: op.URL, Detail: fmt.Sprintf("writing cache file: %v", err),
			Source: op.SrcSpan,
		}
	}

	meta := downloadMeta{
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	op.saveMeta(ctx, src, meta)

	return spec.Result{Changed: true}, nil
}

func (op *DownloadOp) RequiredCapabilities() capability.Capability {
	return capability.None
}

// OpDescription
// -----------------------------------------------------------------------------

type downloadDesc struct {
	URL  string
	Dest string
}

func (d downloadDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   downloadID,
		Text: `download "{{.URL}}" -> "{{.Dest}}"`,
		Data: d,
	}
}

func (op *DownloadOp) OpDescription() spec.OpDescription {
	return downloadDesc{
		URL:  op.URL,
		Dest: op.CachePath,
	}
}

// Helpers
// -----------------------------------------------------------------------------

func (op *DownloadOp) client() *http.Client {
	if op.Client != nil {
		return op.Client
	}
	return http.DefaultClient
}

func (op *DownloadOp) loadMeta(ctx context.Context, src source.Source) downloadMeta {
	data, err := src.ReadFile(ctx, op.metaPath())
	if err != nil {
		return downloadMeta{}
	}
	var meta downloadMeta
	if json.Unmarshal(data, &meta) != nil {
		return downloadMeta{}
	}
	return meta
}

func (op *DownloadOp) saveMeta(ctx context.Context, src source.Source, meta downloadMeta) {
	data, err := json.Marshal(meta)
	if err != nil {
		return
	}
	_ = src.WriteFile(ctx, op.metaPath(), data)
}

// Checksum helpers
// -----------------------------------------------------------------------------

func verifyChecksum(data []byte, cs spec.Checksum) bool {
	h := newHash(cs.Algo)
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil)) == cs.Hex
}

func computeChecksum(data []byte, cs spec.Checksum) string {
	h := newHash(cs.Algo)
	_, _ = h.Write(data)
	return cs.Algo.String() + ":" + hex.EncodeToString(h.Sum(nil))
}

func newHash(algo spec.ChecksumAlgo) hash.Hash {
	switch algo {
	case spec.AlgoSHA384:
		return sha512.New384()
	case spec.AlgoSHA512:
		return sha512.New()
	case spec.AlgoSHA1:
		return sha1.New()
	case spec.AlgoMD5:
		return md5.New()
	default:
		return sha256.New()
	}
}

// Errors
// -----------------------------------------------------------------------------

type DownloadError struct {
	diagnostic.FatalError
	URL    string
	Detail string
	Source spec.SourceSpan
}

func (e DownloadError) Error() string {
	return fmt.Sprintf("download %q: %s", e.URL, e.Detail)
}

func (e DownloadError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeDownloadError,
		Text:   `download "{{.URL}}": {{.Detail}}`,
		Hint:   "check that the URL is reachable and correct",
		Data:   e,
		Source: &e.Source,
	}
}

type ChecksumMismatchError struct {
	diagnostic.FatalError
	URL      string
	Expected string
	Got      string
	Source   spec.SourceSpan
}

func (e ChecksumMismatchError) Error() string {
	return fmt.Sprintf("download %q: checksum mismatch: expected %s, got %s", e.URL, e.Expected, e.Got)
}

func (e ChecksumMismatchError) EventTemplate() event.Template {
	return event.Template{
		ID:     CodeChecksumMismatch,
		Text:   `download "{{.URL}}": checksum mismatch`,
		Hint:   "expected {{.Expected}}, got {{.Got}}",
		Help:   "the downloaded content does not match the declared checksum — verify the URL serves the expected file",
		Data:   e,
		Source: &e.Source,
	}
}
