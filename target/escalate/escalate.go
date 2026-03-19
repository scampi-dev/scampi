// SPDX-License-Identifier: GPL-3.0-only

// Package escalate provides privilege-escalated stat helpers.
// GNU (Linux) and BSD (macOS/FreeBSD) stat formats are both
// supported — callers pick the right variant for their platform.
package escalate

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/target"
)

// GNU format: stat -c '%f %s %Y %n'
// BSD format: stat -f '%Xp %z %m %N'
// Both produce: hex-mode size mtime-epoch name

// Stat runs an escalated stat, picking GNU or BSD format for the platform.
func Stat(
	ctx context.Context, run target.Command, p target.Platform,
	tool, path string, followSymlinks bool,
) (fs.FileInfo, error) {
	switch {
	case p.IsGNU():
		return GNUStat(ctx, run, tool, path, followSymlinks)
	case p.IsBSD():
		return BSDStat(ctx, run, tool, path, followSymlinks)
	default:
		panic(errs.BUG("escalated stat: unsupported platform %q", p))
	}
}

// GetOwner runs an escalated stat to retrieve file ownership.
func GetOwner(
	ctx context.Context, run target.Command, p target.Platform,
	tool, path string,
) (target.Owner, error) {
	switch {
	case p.IsGNU():
		return GNUGetOwner(ctx, run, tool, path)
	case p.IsBSD():
		return BSDGetOwner(ctx, run, tool, path)
	default:
		panic(errs.BUG("escalated get-owner: unsupported platform %q", p))
	}
}

// GNUStat uses GNU stat format (local target, build-tagged).
func GNUStat(ctx context.Context, run target.Command, tool, path string, followSymlinks bool) (fs.FileInfo, error) {
	return stat(ctx, run, tool, path, followSymlinks, "-c", "%f %s %Y %n")
}

// BSDStat uses BSD stat format (local target, build-tagged).
func BSDStat(ctx context.Context, run target.Command, tool, path string, followSymlinks bool) (fs.FileInfo, error) {
	return stat(ctx, run, tool, path, followSymlinks, "-f", "%Xp %z %m %N")
}

// GNUGetOwner uses GNU stat format (local target, build-tagged).
func GNUGetOwner(ctx context.Context, run target.Command, tool, path string) (target.Owner, error) {
	return getOwner(ctx, run, tool, path, "-c", "%U %G")
}

// BSDGetOwner uses BSD stat format (local target, build-tagged).
func BSDGetOwner(ctx context.Context, run target.Command, tool, path string) (target.Owner, error) {
	return getOwner(ctx, run, tool, path, "-f", "%Su %Sg")
}

// Shared implementation
// -----------------------------------------------------------------------------

func stat(
	ctx context.Context,
	run target.Command,
	tool, path string,
	followSymlinks bool,
	fmtFlag, fmtSpec string,
) (fs.FileInfo, error) {
	flag := ""
	if followSymlinks {
		flag = "-L "
	}
	cmd := tool + " stat " + flag + fmtFlag + " '" + fmtSpec + "' " + target.ShellQuote(path)
	result, err := run.RunCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		if containsNotFound(result.Stderr) {
			return nil, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		return nil, target.EscalationError{
			Tool: tool, Op: "stat", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return ParseStatOutput(result.Stdout, path)
}

func getOwner(
	ctx context.Context,
	run target.Command,
	tool, path string,
	fmtFlag, fmtSpec string,
) (target.Owner, error) {
	cmd := tool + " stat -L " + fmtFlag + " '" + fmtSpec + "' " + target.ShellQuote(path)
	result, err := run.RunCommand(ctx, cmd)
	if err != nil {
		return target.Owner{}, err
	}
	if result.ExitCode != 0 {
		if containsNotFound(result.Stderr) {
			return target.Owner{}, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		return target.Owner{}, target.EscalationError{
			Tool: tool, Op: "stat", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	fields := strings.Fields(strings.TrimSpace(result.Stdout))
	if len(fields) != 2 {
		return target.Owner{}, fmt.Errorf("unexpected stat output for %q: %q", path, result.Stdout)
	}
	return target.Owner{User: fields[0], Group: fields[1]}, nil
}

// StatInfo implements fs.FileInfo from parsed stat output.
// Both GNU and BSD variants produce hex-mode, size, mtime-epoch, name.
type StatInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (s StatInfo) Name() string       { return s.name }
func (s StatInfo) Size() int64        { return s.size }
func (s StatInfo) Mode() fs.FileMode  { return s.mode }
func (s StatInfo) ModTime() time.Time { return s.modTime }
func (s StatInfo) IsDir() bool        { return s.mode.IsDir() }
func (s StatInfo) Sys() any           { return nil }

// ParseStatOutput parses "hex-mode size mtime-epoch name" output
// produced by both GNU and BSD stat format strings.
func ParseStatOutput(output, path string) (fs.FileInfo, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 3 {
		return nil, fmt.Errorf("unexpected stat output for %q: %q", path, output)
	}

	rawMode, err := strconv.ParseUint(fields[0], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("cannot parse mode %q for %q: %w", fields[0], path, err)
	}

	size, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("cannot parse size %q for %q: %w", fields[1], path, err)
	}

	epoch, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("cannot parse mtime %q for %q: %w", fields[2], path, err)
	}

	return StatInfo{
		name:    filepath.Base(path),
		size:    size,
		mode:    unixModeToGoMode(uint32(rawMode)),
		modTime: time.Unix(epoch, 0),
	}, nil
}

// unixModeToGoMode converts a raw Unix st_mode value to Go's fs.FileMode.
// The st_mode bit layout is standardized across Linux, macOS, and FreeBSD.
func unixModeToGoMode(raw uint32) fs.FileMode {
	mode := fs.FileMode(raw & 0o777)
	if raw&0o4000 != 0 {
		mode |= fs.ModeSetuid
	}
	if raw&0o2000 != 0 {
		mode |= fs.ModeSetgid
	}
	if raw&0o1000 != 0 {
		mode |= fs.ModeSticky
	}
	switch raw & 0xf000 {
	case 0x4000:
		mode |= fs.ModeDir
	case 0xa000:
		mode |= fs.ModeSymlink
	case 0xc000:
		mode |= fs.ModeSocket
	case 0x2000:
		mode |= fs.ModeCharDevice | fs.ModeDevice
	case 0x6000:
		mode |= fs.ModeDevice
	case 0x1000:
		mode |= fs.ModeNamedPipe
	}
	return mode
}

func containsNotFound(s string) bool {
	return strings.Contains(s, "No such file or directory") ||
		strings.Contains(s, "not found")
}
