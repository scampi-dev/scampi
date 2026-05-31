// SPDX-License-Identifier: GPL-3.0-only

// Command scampi is a decentralized reconciler for bare-metal infrastructure.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func main() {
	log := slogLog{slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	ctx := context.Background()
	err := run(ctx, os.Args[1:], log)
	switch {
	case err == nil:
		return
	case errors.Is(err, ErrSnapshotRejected):
		log.Error(ctx, "snapshot rejected", "err", err)
		os.Exit(2)
	case errors.Is(err, ErrApplyFailed):
		log.Error(ctx, "apply failed", "err", err)
		os.Exit(1)
	default:
		log.Error(ctx, "scampi failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, log Log) error {
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "apply":
		return cmdApply(ctx, args[1:], log)
	default:
		return errUsage
	}
}

var errUsage = errors.New("usage: scampi apply <dir>")

// ErrSnapshotRejected wraps anything that makes the desired-state
// snapshot structurally unusable (parse, schema). Nothing applies.
var ErrSnapshotRejected = errors.New("snapshot rejected")

// ErrApplyFailed wraps per-resource runtime failures. Some resources
// may have landed; the failures aggregate.
var ErrApplyFailed = errors.New("apply failed")

func cmdApply(ctx context.Context, args []string, log Log) error {
	fset := flag.NewFlagSet("apply", flag.ContinueOnError)
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() != 1 {
		return errUsage
	}
	dir := fset.Arg(0)
	resources, err := parseDir(ctx, log, dir)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSnapshotRejected, err)
	}
	if verr := validate(resources); verr != nil {
		return fmt.Errorf("%w: %w", ErrSnapshotRejected, verr)
	}
	if aerr := apply(ctx, resources, log); aerr != nil {
		return fmt.Errorf("%w: %w", ErrApplyFailed, aerr)
	}
	return nil
}

// Log
// -----------------------------------------------------------------------------

// Log is the observability shape passed to apply paths. slog-backed
// today; a richer impl can replace it without changing call sites.
type Log interface {
	Debug(ctx context.Context, msg string, args ...any)
	Info(ctx context.Context, msg string, args ...any)
	Warn(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, args ...any)
}

type slogLog struct{ l *slog.Logger }

func (s slogLog) Debug(ctx context.Context, msg string, args ...any) {
	s.l.DebugContext(ctx, msg, args...)
}

func (s slogLog) Info(ctx context.Context, msg string, args ...any) {
	s.l.InfoContext(ctx, msg, args...)
}

func (s slogLog) Warn(ctx context.Context, msg string, args ...any) {
	s.l.WarnContext(ctx, msg, args...)
}

func (s slogLog) Error(ctx context.Context, msg string, args ...any) {
	s.l.ErrorContext(ctx, msg, args...)
}

// Resource
// -----------------------------------------------------------------------------

// Resource is one parsed top-level HCL block.
type Resource struct {
	Kind  string
	Name  string
	Attrs map[string]string
}

func (r Resource) Ref() string { return r.Kind + "." + r.Name }

// Parse
// -----------------------------------------------------------------------------

func parseDir(ctx context.Context, log Log, dir string) ([]Resource, error) {
	log.Debug(ctx, "parsing", "dir", dir)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", dir)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.hcl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []Resource
	for _, p := range paths {
		rs, perr := parseFile(ctx, log, p)
		if perr != nil {
			return nil, perr
		}
		out = append(out, rs...)
	}
	return out, nil
}

func parseFile(ctx context.Context, log Log, path string) ([]Resource, error) {
	log.Debug(ctx, "parsing", "path", path)
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, diags := hclsyntax.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, diags
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("%s: unexpected body type %T", path, file.Body)
	}
	if len(body.Attributes) > 0 {
		return nil, fmt.Errorf("%s: top-level attributes not allowed; use blocks", path)
	}
	out := make([]Resource, 0, len(body.Blocks))
	for _, block := range body.Blocks {
		r, perr := parseBlock(ctx, log, block, path)
		if perr != nil {
			return nil, perr
		}
		out = append(out, r)
	}
	return out, nil
}

func parseBlock(ctx context.Context, log Log, block *hclsyntax.Block, path string) (Resource, error) {
	log.Debug(ctx, "parsing", "block", block, "path", path)
	if len(block.Labels) != 1 {
		return Resource{}, fmt.Errorf("%s: %s block needs exactly one label, got %d",
			path, block.Type, len(block.Labels))
	}
	if len(block.Body.Blocks) > 0 {
		return Resource{}, fmt.Errorf("%s: nested blocks not supported", path)
	}
	attrs := make(map[string]string, len(block.Body.Attributes))
	for name, attr := range block.Body.Attributes {
		val, diags := attr.Expr.Value(nil)
		if diags.HasErrors() {
			return Resource{}, diags
		}
		if val.Type() != cty.String {
			return Resource{}, fmt.Errorf("%s:%d: attr %q must be a string",
				path, attr.Range().Start.Line, name)
		}
		attrs[name] = val.AsString()
	}
	return Resource{Kind: block.Type, Name: block.Labels[0], Attrs: attrs}, nil
}

// Validate
// -----------------------------------------------------------------------------

// validate runs per-Kind schema checks across the whole snapshot.
// Aggregates so the operator sees every schema fault in one pass.
func validate(resources []Resource) error {
	var errs []error
	for _, r := range resources {
		if err := validateOne(r); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func validateOne(r Resource) error {
	switch r.Kind {
	case "file":
		return validateFile(r)
	case "dir":
		return validateDir(r)
	default:
		return fmt.Errorf("%s: unknown kind", r.Ref())
	}
}

func validateFile(r Resource) error {
	var errs []error
	if _, ok := r.Attrs["path"]; !ok {
		errs = append(errs, fmt.Errorf("%s: missing required attr %q", r.Ref(), "path"))
	}
	if _, ok := r.Attrs["content"]; !ok {
		errs = append(errs, fmt.Errorf("%s: missing required attr %q", r.Ref(), "content"))
	}
	return errors.Join(errs...)
}

func validateDir(r Resource) error {
	if _, ok := r.Attrs["path"]; !ok {
		return fmt.Errorf("%s: missing required attr %q", r.Ref(), "path")
	}
	return nil
}

// Apply
// -----------------------------------------------------------------------------

func apply(ctx context.Context, resources []Resource, log Log) error {
	var errs []error
	for _, r := range resources {
		if err := applyOne(ctx, r, log); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func applyOne(ctx context.Context, r Resource, log Log) error {
	switch r.Kind {
	case "file":
		return applyFile(ctx, r, log)
	case "dir":
		return applyDir(ctx, r, log)
	default:
		return fmt.Errorf("%s: unknown kind", r.Ref())
	}
}

func applyFile(ctx context.Context, r Resource, log Log) error {
	// path and content are required; schema validation enforces this
	// before any apply runs.
	path := r.Attrs["path"]
	content := r.Attrs["content"]
	current, rerr := os.ReadFile(path)
	switch {
	case rerr == nil && string(current) == content:
		log.Debug(ctx, "file in sync", "ref", r.Ref(), "path", path)
		return nil
	case rerr != nil && !errors.Is(rerr, fs.ErrNotExist):
		return fmt.Errorf("%s: read %s: %w", r.Ref(), path, rerr)
	}
	log.Info(ctx, "writing file", "ref", r.Ref(), "path", path)
	if werr := os.WriteFile(path, []byte(content), 0o644); werr != nil {
		return fmt.Errorf("%s: write %s: %w", r.Ref(), path, werr)
	}
	return nil
}

func applyDir(ctx context.Context, r Resource, log Log) error {
	// path is required; schema validation enforces this before any
	// apply runs.
	path := r.Attrs["path"]
	info, serr := os.Stat(path)
	switch {
	case serr == nil && info.IsDir():
		log.Debug(ctx, "dir in sync", "ref", r.Ref(), "path", path)
		return nil
	case serr == nil:
		return fmt.Errorf("%s: %s exists but is not a directory", r.Ref(), path)
	case !errors.Is(serr, fs.ErrNotExist):
		return fmt.Errorf("%s: stat %s: %w", r.Ref(), path, serr)
	}
	log.Info(ctx, "creating dir", "ref", r.Ref(), "path", path)
	if merr := os.MkdirAll(path, 0o755); merr != nil {
		return fmt.Errorf("%s: mkdir %s: %w", r.Ref(), path, merr)
	}
	return nil
}
