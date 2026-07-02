// SPDX-License-Identifier: GPL-3.0-only

// Escalated fallback methods: privileged command construction for the
// POSIXTarget mutations that hit a permission wall.

package local

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"

	"scampi.dev/scampi/internal/target"
)

func (t POSIXTarget) escalatedReadFile(ctx context.Context, path string) ([]byte, error) {
	result, err := t.RunCommand(ctx, t.Escalate+" cat "+target.ShellQuote(path))
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, target.EscalationError{
			Tool: t.Escalate, Op: "cat", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return []byte(result.Stdout), nil
}

func (t POSIXTarget) escalatedWriteFile(ctx context.Context, path string, data []byte) error {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return target.StagingError{Path: path, Err: err}
	}
	tmp := "/tmp/.scampi-" + hex.EncodeToString(buf[:])

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return target.StagingError{Path: path, Err: err}
	}
	defer func() { _ = os.Remove(tmp) }()

	result, err := t.RunCommand(ctx, t.Escalate+" cp "+target.ShellQuote(tmp)+" "+target.ShellQuote(path))
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.Escalate, Op: "cp", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t POSIXTarget) escalatedRemove(ctx context.Context, path string) error {
	result, err := t.RunCommand(ctx, t.Escalate+" rm "+target.ShellQuote(path))
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.Escalate, Op: "rm", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t POSIXTarget) escalatedChmod(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := t.Escalate + " chmod " + octal + " " + target.ShellQuote(path)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.Escalate, Op: "chmod", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t POSIXTarget) escalatedChown(ctx context.Context, path string, owner target.Owner) error {
	cmd := t.Escalate + " chown " +
		target.ShellQuote(owner.User) + ":" + target.ShellQuote(owner.Group) +
		" " + target.ShellQuote(path)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.Escalate, Op: "chown", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t POSIXTarget) escalatedSymlink(ctx context.Context, tgt, link string) error {
	cmd := t.Escalate + " ln -sfn " +
		target.ShellQuote(tgt) + " " + target.ShellQuote(link)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.Escalate, Op: "ln", Path: link,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t POSIXTarget) escalatedMkdir(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := t.Escalate + " mkdir -p " + target.ShellQuote(path) +
		" && " + t.Escalate + " chmod " + octal + " " + target.ShellQuote(path)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.Escalate, Op: "mkdir", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}
