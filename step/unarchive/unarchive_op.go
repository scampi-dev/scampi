// SPDX-License-Identifier: GPL-3.0-only

package unarchive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/step/sharedops"
	"scampi.dev/scampi/target"
)

const unarchiveID = "builtin.unarchive"

const maxStderrLines = 10

func truncateStderr(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= maxStderrLines {
		return s
	}
	kept := strings.Join(lines[:maxStderrLines], "\n")
	return fmt.Sprintf("%s\n... (%d more lines)", kept, len(lines)-maxStderrLines)
}

const stateDir = "/var/lib/scampi/unarchive"

type unarchiveOp struct {
	sharedops.BaseOp
	src    string
	srcRef spec.SourceRef
	dest   string
	depth  int
	format archiveFormat
}

func markerPath(dest string) string {
	h := sha256.Sum256([]byte(dest))
	return stateDir + "/" + hex.EncodeToString(h[:]) + ".sha256"
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// Check
// -----------------------------------------------------------------------------

func (op *unarchiveOp) Check(
	ctx context.Context,
	src source.Source,
	tgt target.Target,
) (spec.CheckResult, []spec.DriftDetail, error) {
	fsTgt := target.Must[target.Filesystem](unarchiveID, tgt)

	srcData, err := src.ReadFile(ctx, op.src)
	if err != nil {
		if result, drift, ok := sharedops.CheckSourcePending(op.srcRef, "archive"); ok {
			return result, drift, nil
		}
		return spec.CheckUnsatisfied, nil, ArchiveNotFoundError{
			Path:   op.src,
			Err:    err,
			Source: op.SrcSpan,
		}
	}

	srcHash := hashBytes(srcData)
	marker := markerPath(op.dest)

	markerData, err := fsTgt.ReadFile(ctx, marker)
	if err != nil {
		return spec.CheckUnsatisfied, []spec.DriftDetail{{
			Field:   "archive",
			Desired: srcHash[:12],
		}}, nil
	}

	if strings.TrimSpace(string(markerData)) == srcHash {
		return spec.CheckSatisfied, nil, nil
	}

	return spec.CheckUnsatisfied, []spec.DriftDetail{{
		Field:   "archive",
		Current: strings.TrimSpace(string(markerData))[:12],
		Desired: srcHash[:12],
	}}, nil
}

// Execute
// -----------------------------------------------------------------------------

func (op *unarchiveOp) Execute(
	ctx context.Context,
	src source.Source,
	tgt target.Target,
) (spec.Result, error) {
	fsTgt := target.Must[target.Filesystem](unarchiveID, tgt)
	cmdTgt := target.Must[target.Command](unarchiveID, tgt)

	srcData, err := src.ReadFile(ctx, op.src)
	if err != nil {
		return spec.Result{}, ArchiveNotFoundError{
			Path:   op.src,
			Err:    err,
			Source: op.SrcSpan,
		}
	}

	srcHash := hashBytes(srcData)

	// Idempotency re-check
	marker := markerPath(op.dest)
	markerData, err := fsTgt.ReadFile(ctx, marker)
	if err == nil && strings.TrimSpace(string(markerData)) == srcHash {
		return spec.Result{Changed: false}, nil
	}

	// Ensure dest and state dirs exist
	if err := fsTgt.Mkdir(ctx, op.dest, 0o755); err != nil {
		if target.IsPermission(err) {
			return spec.Result{}, sharedops.PermissionDeniedError{
				Operation: "mkdir " + op.dest,
				Source:    op.DestSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}
	if err := fsTgt.Mkdir(ctx, stateDir, 0o755); err != nil {
		if target.IsPermission(err) {
			return spec.Result{}, sharedops.PermissionDeniedError{
				Operation: "mkdir " + stateDir,
				Source:    op.DestSpan,
				Err:       err,
			}
		}
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}

	// Extract
	if err := op.extract(ctx, fsTgt, cmdTgt, srcData); err != nil {
		return spec.Result{}, err
	}

	// Recursive nested unpacking
	if op.depth != 0 {
		if err := op.extractNested(ctx, cmdTgt, op.depth); err != nil {
			return spec.Result{}, err
		}
	}

	// Write marker
	if err := fsTgt.WriteFile(ctx, marker, []byte(srcHash+"\n")); err != nil {
		return spec.Result{}, sharedops.DiagnoseTargetError(err)
	}

	return spec.Result{Changed: true}, nil
}

func (op *unarchiveOp) RequiredCapabilities() capability.Capability {
	return capability.Filesystem | capability.Command
}

// Extraction backends
// -----------------------------------------------------------------------------

func (op *unarchiveOp) extract(
	ctx context.Context,
	fsTgt target.Filesystem,
	cmdTgt target.Command,
	data []byte,
) error {
	if op.probeToolAvailable(ctx, cmdTgt) {
		return op.extractWithTool(ctx, fsTgt, cmdTgt, data)
	}

	return op.extractWithGo(ctx, fsTgt, data)
}

func (op *unarchiveOp) probeToolAvailable(ctx context.Context, cmdTgt target.Command) bool {
	tool := op.format.requiredTool()
	res, err := cmdTgt.RunCommand(ctx, "command -v "+tool)
	return err == nil && res.ExitCode == 0
}

func (op *unarchiveOp) extractWithTool(
	ctx context.Context,
	fsTgt target.Filesystem,
	cmdTgt target.Command,
	data []byte,
) error {
	tmpPath := "/tmp/.scampi-unarchive-" + hashBytes(data)[:16] + filepath.Ext(op.src)

	if err := fsTgt.WriteFile(ctx, tmpPath, data); err != nil {
		return sharedops.DiagnoseTargetError(err)
	}

	cmd := op.format.extractCmd(tmpPath, op.dest)
	res, err := cmdTgt.RunPrivileged(ctx, cmd)

	// Always clean up temp file
	_ = fsTgt.Remove(ctx, tmpPath)

	if err != nil {
		return sharedops.DiagnoseTargetError(err)
	}
	if res.ExitCode != 0 {
		return ExtractionError{
			Cmd:    cmd,
			Stderr: truncateStderr(res.Stderr),
			Advice: extractionAdvice(res.Stderr),
			Source: op.SrcSpan,
		}
	}

	return nil
}

func (op *unarchiveOp) extractWithGo(
	ctx context.Context,
	fsTgt target.Filesystem,
	data []byte,
) error {
	switch op.format {
	case formatZip:
		return extractZip(ctx, fsTgt, data, op.dest)
	case formatTarGz:
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return ExtractionError{
				Cmd:    "gzip decompress",
				Stderr: err.Error(),
				Advice: extractionAdvice(err.Error()),
				Source: op.SrcSpan,
			}
		}
		defer func() { _ = r.Close() }()
		return extractTar(ctx, fsTgt, r, op.dest)
	case formatTarBz2:
		r := bzip2.NewReader(bytes.NewReader(data))
		return extractTar(ctx, fsTgt, r, op.dest)
	case formatTarXz:
		r, err := xz.NewReader(bytes.NewReader(data))
		if err != nil {
			return ExtractionError{
				Cmd:    "xz decompress",
				Stderr: err.Error(),
				Advice: extractionAdvice(err.Error()),
				Source: op.SrcSpan,
			}
		}
		return extractTar(ctx, fsTgt, r, op.dest)
	case formatTarZst:
		d, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return ExtractionError{
				Cmd:    "zstd decompress",
				Stderr: err.Error(),
				Advice: extractionAdvice(err.Error()),
				Source: op.SrcSpan,
			}
		}
		defer d.Close()
		return extractTar(ctx, fsTgt, d, op.dest)
	case formatTar:
		return extractTar(ctx, fsTgt, bytes.NewReader(data), op.dest)
	default:
		panic(errs.BUG("unhandled archive format %d", op.format))
	}
}

func extractTar(ctx context.Context, fsTgt target.Filesystem, r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		path := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dest)) {
			continue // skip entries that escape dest (zip-slip protection)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = fsTgt.Mkdir(ctx, path, hdr.FileInfo().Mode())
		case tar.TypeReg:
			dir := filepath.Dir(path)
			_ = fsTgt.Mkdir(ctx, dir, 0o755)
			data, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("reading tar entry %q: %w", hdr.Name, err)
			}
			if err := fsTgt.WriteFile(ctx, path, data); err != nil {
				return err
			}
		}
	}
}

func extractZip(ctx context.Context, fsTgt target.Filesystem, data []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("reading zip: %w", err)
	}

	for _, f := range zr.File {
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dest)) {
			continue // zip-slip protection
		}

		if f.FileInfo().IsDir() {
			_ = fsTgt.Mkdir(ctx, path, f.Mode())
			continue
		}

		dir := filepath.Dir(path)
		_ = fsTgt.Mkdir(ctx, dir, 0o755)

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("opening zip entry %q: %w", f.Name, err)
		}
		fileData, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return fmt.Errorf("reading zip entry %q: %w", f.Name, err)
		}
		if err := fsTgt.WriteFile(ctx, path, fileData); err != nil {
			return err
		}
	}

	return nil
}

// Nested extraction
// -----------------------------------------------------------------------------

func (op *unarchiveOp) extractNested(
	ctx context.Context,
	cmdTgt target.Command,
	remaining int,
) error {
	findCmd := fmt.Sprintf("find %s -type f \\( %s \\)", op.dest, archiveExtensions())
	res, err := cmdTgt.RunPrivileged(ctx, findCmd)
	if err != nil {
		return sharedops.DiagnoseTargetError(err)
	}
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	for _, archivePath := range lines {
		archivePath = strings.TrimSpace(archivePath)
		if archivePath == "" {
			continue
		}

		nestedFmt, ok := detectFormat(archivePath)
		if !ok {
			continue
		}

		parentDir := filepath.Dir(archivePath)
		cmd := nestedFmt.extractCmd(archivePath, parentDir)
		extractRes, err := cmdTgt.RunPrivileged(ctx, cmd)
		if err != nil {
			return sharedops.DiagnoseTargetError(err)
		}
		if extractRes.ExitCode != 0 {
			return ExtractionError{
				Cmd:    cmd,
				Stderr: truncateStderr(extractRes.Stderr),
				Advice: extractionAdvice(extractRes.Stderr),
				Source: op.DestSpan,
			}
		}

		// Remove nested archive
		rmCmd := "rm " + archivePath
		rmRes, err := cmdTgt.RunPrivileged(ctx, rmCmd)
		if err != nil {
			return sharedops.DiagnoseTargetError(err)
		}
		if rmRes.ExitCode != 0 {
			return ExtractionError{
				Cmd:    rmCmd,
				Stderr: truncateStderr(rmRes.Stderr),
				Advice: extractionAdvice(rmRes.Stderr),
				Source: op.DestSpan,
			}
		}
	}

	if remaining == -1 || remaining > 1 {
		next := remaining
		if next > 0 {
			next--
		}
		return op.extractNested(ctx, cmdTgt, next)
	}

	return nil
}

// OpDescription
// -----------------------------------------------------------------------------

type unarchiveDesc struct {
	Src  string
	Dest string
}

func (d unarchiveDesc) PlanTemplate() spec.PlanTemplate {
	return spec.PlanTemplate{
		ID:   unarchiveID,
		Text: `unarchive "{{.Src}}" -> "{{.Dest}}"`,
		Data: d,
	}
}

func (op *unarchiveOp) OpDescription() spec.OpDescription {
	return unarchiveDesc{
		Src:  op.src,
		Dest: op.dest,
	}
}
