// SPDX-License-Identifier: GPL-3.0-only

package pve

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/escalate"
	"scampi.dev/scampi/target/posix"
)

// LXCTarget runs operations inside an LXC container by SSHing to
// the PVE host and proxying commands/files through pct exec/push.
type LXCTarget struct {
	posix.Base
	config     *Config
	client     *ssh.Client
	sftp       *sftp.Client
	closeAgent func() error
	vmid       int

	// Host-side escalation (for invoking pct as root from a non-root user).
	hostIsRoot   bool
	hostEscalate string
}

func (t *LXCTarget) Close() {
	if t.closeAgent != nil {
		_ = t.closeAgent()
	}
	if t.sftp != nil {
		_ = t.sftp.Close()
	}
	if t.client != nil {
		_ = t.client.Close()
	}
}

// Command
// -----------------------------------------------------------------------------

// runOnHost executes a command directly on the PVE host (no pct wrapping).
// Used for setup probes (id -u, command -v sudo, etc.).
func (t *LXCTarget) runOnHost(_ context.Context, cmd string) (target.CommandResult, error) {
	session, err := t.client.NewSession()
	if err != nil {
		return target.CommandResult{}, errs.WrapErrf(err, "ssh session")
	}
	defer func() { _ = session.Close() }()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(cmd)
	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return target.CommandResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: exitErr.ExitStatus(),
			}, nil
		}
		return target.CommandResult{}, errs.WrapErrf(err, "ssh run")
	}
	return target.CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}, nil
}

// RunCommand satisfies target.Command — runs a command inside the
// container via pct exec.
func (t *LXCTarget) RunCommand(ctx context.Context, cmd string) (target.CommandResult, error) {
	return t.runInContainer(ctx, cmd)
}

// runInContainer executes a command inside the container via pct exec.
// This is the Runner that posix.Base uses for in-container operations.
func (t *LXCTarget) runInContainer(ctx context.Context, cmd string) (target.CommandResult, error) {
	// pct exec interprets the command via execve, so we wrap it in
	// `sh -c` to support shell features (pipes, redirects, etc.) that
	// step code generates.
	wrapped := fmt.Sprintf("%s sh -c %s", t.pctPrefix()+fmt.Sprintf(" exec %d --", t.vmid), target.ShellQuote(cmd))
	return t.runOnHost(ctx, wrapped)
}

// Filesystem
// -----------------------------------------------------------------------------

func (t *LXCTarget) ReadFile(ctx context.Context, path string) ([]byte, error) {
	// pct pull writes the file to a host path; we then SFTP-read it
	// and clean up. This avoids stdout binary-corruption issues.
	tmp, err := t.hostTempPath()
	if err != nil {
		return nil, err
	}
	defer func() { _ = t.sftp.Remove(tmp) }()

	pullCmd := fmt.Sprintf("%s pull %d %s %s",
		t.pctPrefix(), t.vmid, target.ShellQuote(path), target.ShellQuote(tmp))
	r, err := t.runOnHost(ctx, pullCmd)
	if err != nil {
		return nil, err
	}
	if r.ExitCode != 0 {
		if isPctNotFound(r.Stderr) {
			return nil, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		return nil, PctFailedError{
			Op: "pull", VMID: t.vmid,
			ExitCode: r.ExitCode, Stderr: r.Stderr,
		}
	}

	// pct pull may chown the file to the container's UID — readable
	// by us since we run sudo. SFTP from our user might fail; fall
	// back to `cat` via host runner.
	f, ferr := t.sftp.Open(tmp)
	if ferr == nil {
		defer func() { _ = f.Close() }()
		return io.ReadAll(f)
	}
	// Fallback: cat via host (with sudo if needed).
	catCmd := "cat " + target.ShellQuote(tmp)
	if t.hostEscalate != "" {
		catCmd = t.hostEscalate + " " + catCmd
	}
	r, err = t.runOnHost(ctx, catCmd)
	if err != nil {
		return nil, err
	}
	if r.ExitCode != 0 {
		// bare-error: host-side cat fallback after pct pull
		return nil, errs.Errorf("read %s: %s", path, r.Stderr)
	}
	return []byte(r.Stdout), nil
}

func (t *LXCTarget) WriteFile(ctx context.Context, path string, data []byte) error {
	// Stage the file on the PVE host via SFTP, then pct push it into
	// the container, then clean up.
	tmp, err := t.writeTempFile(data)
	if err != nil {
		return errs.WrapErrf(err, "stage %s", path)
	}
	defer func() { _ = t.sftp.Remove(tmp) }()

	pushCmd := fmt.Sprintf("%s push %d %s %s",
		t.pctPrefix(), t.vmid, target.ShellQuote(tmp), target.ShellQuote(path))
	r, err := t.runOnHost(ctx, pushCmd)
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return PctFailedError{
			Op: "push", VMID: t.vmid,
			ExitCode: r.ExitCode, Stderr: r.Stderr,
		}
	}
	return nil
}

func (t *LXCTarget) Stat(ctx context.Context, path string) (fs.FileInfo, error) {
	cmd := fmt.Sprintf("stat -L -c '%%f %%s %%Y %%n' %s", target.ShellQuote(path))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if r.ExitCode != 0 {
		if containsNotFound(r.Stderr) {
			return nil, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		return nil, PctFailedError{Op: "stat", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return escalate.ParseStatOutput(r.Stdout, path)
}

func (t *LXCTarget) Lstat(ctx context.Context, path string) (fs.FileInfo, error) {
	cmd := fmt.Sprintf("stat -c '%%f %%s %%Y %%n' %s", target.ShellQuote(path))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if r.ExitCode != 0 {
		if containsNotFound(r.Stderr) {
			return nil, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		return nil, PctFailedError{Op: "lstat", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return escalate.ParseStatOutput(r.Stdout, path)
}

func (t *LXCTarget) ReadDir(ctx context.Context, path string) ([]fs.DirEntry, error) {
	// `ls -1A` lists names without ./.., one per line.
	cmd := fmt.Sprintf("ls -1A %s", target.ShellQuote(path))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if r.ExitCode != 0 {
		if containsNotFound(r.Stderr) {
			return nil, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		return nil, PctFailedError{Op: "readdir", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	var entries []fs.DirEntry
	for _, name := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		if name == "" {
			continue
		}
		info, err := t.Stat(ctx, path+"/"+name)
		if err != nil {
			return nil, err
		}
		entries = append(entries, fs.FileInfoToDirEntry(info))
	}
	return entries, nil
}

func (t *LXCTarget) Mkdir(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := fmt.Sprintf("mkdir -p %s && chmod %s %s",
		target.ShellQuote(path), octal, target.ShellQuote(path))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return PctFailedError{Op: "mkdir", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return nil
}

func (t *LXCTarget) Remove(ctx context.Context, path string) error {
	cmd := fmt.Sprintf("rm -f %s", target.ShellQuote(path))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return PctFailedError{Op: "remove", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return nil
}

// FileMode
// -----------------------------------------------------------------------------

func (t *LXCTarget) Chmod(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := fmt.Sprintf("chmod %s %s", octal, target.ShellQuote(path))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return PctFailedError{Op: "chmod", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return nil
}

// Symlinks
// -----------------------------------------------------------------------------

func (t *LXCTarget) Readlink(ctx context.Context, path string) (string, error) {
	cmd := fmt.Sprintf("readlink %s", target.ShellQuote(path))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return "", err
	}
	if r.ExitCode != 0 {
		if containsNotFound(r.Stderr) {
			return "", errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		return "", PctFailedError{Op: "readlink", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return strings.TrimRight(r.Stdout, "\n"), nil
}

func (t *LXCTarget) Symlink(ctx context.Context, tgt, link string) error {
	cmd := fmt.Sprintf("ln -sfn %s %s", target.ShellQuote(tgt), target.ShellQuote(link))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return PctFailedError{Op: "symlink", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return nil
}

// Ownership
// -----------------------------------------------------------------------------

func (t *LXCTarget) HasUser(ctx context.Context, user string) bool {
	r, err := t.runInContainer(ctx, fmt.Sprintf("id -u %s", target.ShellQuote(user)))
	return err == nil && r.ExitCode == 0
}

func (t *LXCTarget) HasGroup(ctx context.Context, group string) bool {
	r, err := t.runInContainer(ctx, fmt.Sprintf("getent group %s", target.ShellQuote(group)))
	return err == nil && r.ExitCode == 0
}

func (t *LXCTarget) GetOwner(ctx context.Context, path string) (target.Owner, error) {
	cmd := fmt.Sprintf("stat -L -c '%%U %%G' %s", target.ShellQuote(path))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return target.Owner{}, err
	}
	if r.ExitCode != 0 {
		if containsNotFound(r.Stderr) {
			return target.Owner{}, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		return target.Owner{}, PctFailedError{Op: "stat", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	fields := strings.Fields(strings.TrimSpace(r.Stdout))
	if len(fields) != 2 {
		// bare-error: parse failure on stat output
		return target.Owner{}, errs.Errorf("unexpected stat output for %q: %q", path, r.Stdout)
	}
	return target.Owner{User: fields[0], Group: fields[1]}, nil
}

func (t *LXCTarget) Chown(ctx context.Context, path string, owner target.Owner) error {
	cmd := fmt.Sprintf("chown %s:%s %s",
		target.ShellQuote(owner.User), target.ShellQuote(owner.Group), target.ShellQuote(path))
	r, err := t.runInContainer(ctx, cmd)
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return PctFailedError{Op: "chown", VMID: t.vmid, ExitCode: r.ExitCode, Stderr: r.Stderr}
	}
	return nil
}

// Helpers
// -----------------------------------------------------------------------------

func (t *LXCTarget) hostTempPath() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "/tmp/.scampi-pve-" + hex.EncodeToString(buf[:]), nil
}

func (t *LXCTarget) writeTempFile(data []byte) (string, error) {
	tmp, err := t.hostTempPath()
	if err != nil {
		return "", err
	}
	f, err := t.sftp.Create(tmp)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		_ = t.sftp.Remove(tmp)
		return "", err
	}
	return tmp, nil
}

// containsNotFound mirrors the helper in escalate. pct exec returns
// the container's stat stderr, which on Linux says "No such file
// or directory".
func containsNotFound(s string) bool {
	return strings.Contains(s, "No such file or directory") ||
		strings.Contains(s, "not found")
}

// isPctNotFound detects "pct pull"-style errors when the source path
// inside the container doesn't exist.
func isPctNotFound(s string) bool {
	return strings.Contains(s, "No such file or directory") ||
		strings.Contains(s, "does not exist")
}
