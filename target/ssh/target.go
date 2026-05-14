// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/escalate"
	"scampi.dev/scampi/target/posix"
)

// Connection pooling, sanity cap, and resilience
// -----------------------------------------------------------------------------
//
// SSHTarget pools its TCP/SSH connection at the *target* level: one
// `*ssh.Client` per SSHTarget instance, created once in `SSH.Create()`
// and shared for the target's lifetime.
//
// On top of that connection sits a pool of long-lived SHELL SESSIONS
// (see shell_pool.go and shell_session.go). Each pooled session is
// one `/bin/sh` running on the target; commands are streamed into
// the shell's stdin and parsed back out of stdout. This amortises
// the channel-open + exec handshake (~2 RTTs) over the lifetime of
// the session — every op past the first pays only the command's own
// RTT. Critical at WAN latency.
//
// The pool caps concurrent in-flight sessions at MaxSessions
// (default 10, matching OpenSSH's sshd_config default), absorbs
// server-side backpressure with exponential-backoff retry, and
// drops sessions that fail any I/O (the next acquire opens a fresh
// one).
//
// The same single-connection model applies to the SFTP subsystem:
// one `*sftp.Client` reuses the SSH connection for all file
// operations.
//
// Stats() exposes counters so the pooling invariants can be tested
// and any regression (a stray re-dial, leak, unexpected queue
// depth) is visible.

// DefaultMaxSessions is the pool size used when Config.MaxSessions
// is unset. Matches OpenSSH's sshd_config default of MaxSessions=10
// — conservative enough to never hammer a stock server.
const DefaultMaxSessions = 10

// SSHStats reports per-target SSH usage counters.
//
//   - DialCount stays at 1 for the target's lifetime.
//   - MaxSessions is the configured pool capacity.
//   - SessionsOpened increments once per shell session created (now
//     bounded by MaxSessions instead of by command count).
//   - SessionsPeakInFlight tracks the high-water mark of concurrent
//     sessions executing commands at the same instant.
//   - SessionRetries counts how many times opening a shell was
//     retried after the server rejected the channel open. Non-zero
//     means we hit server-side rate limits — diagnostic for tuning
//     MaxSessions.
//   - CommandsRun is the total RunCommand calls served. Compare with
//     SessionsOpened to see the reuse ratio.
type SSHStats struct {
	DialCount            int64
	MaxSessions          int64
	SessionsOpened       int64
	SessionsInFlight     int64 // open + currently checked out
	SessionsAcquired     int64 // semaphore tokens held (includes retry-to-open waiters)
	SessionsPeakInFlight int64
	SessionRetries       int64
	CommandsRun          int64
}

type SSHTarget struct {
	posix.Base
	config     *Config
	client     *ssh.Client
	sftp       *sftp.Client
	closeAgent func() error

	pool      *shellPool
	dialCount atomic.Int64
}

// Stats returns a snapshot of SSH usage counters for this target.
func (t *SSHTarget) Stats() SSHStats {
	return SSHStats{
		DialCount:            t.dialCount.Load(),
		MaxSessions:          int64(t.pool.capacity),
		SessionsOpened:       t.pool.sessionsOpened.Load(),
		SessionsInFlight:     t.pool.sessionsInFlight.Load(),
		SessionsAcquired:     int64(len(t.pool.sem)),
		SessionsPeakInFlight: t.pool.sessionsPeakSeen.Load(),
		SessionRetries:       t.pool.sessionRetries.Load(),
		CommandsRun:          t.pool.commandsRun.Load(),
	}
}

func (t *SSHTarget) Close() {
	if t.pool != nil {
		t.pool.closeAll()
	}
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

// Filesystem
// -----------------------------------------------------------------------------

func (t *SSHTarget) ReadFile(ctx context.Context, path string) ([]byte, error) {
	f, err := t.sftp.Open(path)
	if err != nil {
		if isPermission(err) {
			if t.Escalate != "" {
				return t.escalatedReadFile(ctx, path)
			}
			if !t.IsRoot {
				return nil, target.NoEscalationError{Op: "read", Path: path}
			}
		}
		return nil, normalizeError(err)
	}
	defer func() { _ = f.Close() }()

	res, err := io.ReadAll(f)
	return res, normalizeError(err)
}

func (t *SSHTarget) ReadDir(_ context.Context, path string) ([]fs.DirEntry, error) {
	infos, err := t.sftp.ReadDir(path)
	if err != nil {
		return nil, normalizeError(err)
	}
	entries := make([]fs.DirEntry, len(infos))
	for i, info := range infos {
		entries[i] = fs.FileInfoToDirEntry(info)
	}
	return entries, nil
}

func (t *SSHTarget) WriteFile(ctx context.Context, path string, data []byte) error {
	f, err := t.sftp.Create(path)
	if err != nil {
		if isPermission(err) {
			if t.Escalate != "" {
				return t.escalatedWriteFile(ctx, path, data)
			}
			if !t.IsRoot {
				return target.NoEscalationError{Op: "write", Path: path}
			}
		}
		return normalizeError(err)
	}
	defer func() { _ = f.Close() }()

	_, err = f.Write(data)
	return normalizeError(err)
}

func (t *SSHTarget) Stat(ctx context.Context, path string) (fs.FileInfo, error) {
	info, err := t.sftp.Stat(path)
	if err != nil {
		if isPermission(err) && t.Escalate != "" {
			return t.escalatedStat(ctx, path, true)
		}
		return nil, normalizeError(err)
	}
	return info, nil
}

func (t *SSHTarget) Remove(ctx context.Context, path string) error {
	err := t.sftp.Remove(path)
	if isPermission(err) {
		if t.Escalate != "" {
			return t.escalatedRemove(ctx, path)
		}
		if !t.IsRoot {
			return target.NoEscalationError{Op: "remove", Path: path}
		}
	}
	return normalizeError(err)
}

func (t *SSHTarget) Mkdir(ctx context.Context, path string, mode fs.FileMode) error {
	err := t.sftp.MkdirAll(path)
	if err != nil {
		if isPermission(err) {
			if t.Escalate != "" {
				return t.escalatedMkdir(ctx, path, mode)
			}
			if !t.IsRoot {
				return target.NoEscalationError{Op: "mkdir", Path: path}
			}
		}
		return normalizeError(err)
	}
	if err := t.sftp.Chmod(path, mode); err != nil {
		if isPermission(err) {
			if t.Escalate != "" {
				return t.escalatedMkdir(ctx, path, mode)
			}
			if !t.IsRoot {
				return target.NoEscalationError{Op: "chmod", Path: path}
			}
		}
		return normalizeError(err)
	}
	return nil
}

// FileMode
// -----------------------------------------------------------------------------

func (t *SSHTarget) Chmod(ctx context.Context, path string, mode fs.FileMode) error {
	err := t.sftp.Chmod(path, mode)
	if isPermission(err) {
		if t.Escalate != "" {
			return t.escalatedChmod(ctx, path, mode)
		}
		if !t.IsRoot {
			return target.NoEscalationError{Op: "chmod", Path: path}
		}
	}
	return normalizeError(err)
}

// Symlinks
// -----------------------------------------------------------------------------

func (t *SSHTarget) Lstat(ctx context.Context, path string) (fs.FileInfo, error) {
	info, err := t.sftp.Lstat(path)
	if err != nil {
		if isPermission(err) && t.Escalate != "" {
			return t.escalatedStat(ctx, path, false)
		}
		return nil, normalizeError(err)
	}
	return info, nil
}

func (t *SSHTarget) Readlink(_ context.Context, path string) (string, error) {
	res, err := t.sftp.ReadLink(path)
	return res, normalizeError(err)
}

func (t *SSHTarget) Symlink(ctx context.Context, tgt, link string) error {
	err := t.sftp.Symlink(tgt, link)
	if isPermission(err) {
		if t.Escalate != "" {
			return t.escalatedSymlink(ctx, tgt, link)
		}
		if !t.IsRoot {
			return target.NoEscalationError{Op: "symlink", Path: link}
		}
	}
	return normalizeError(err)
}

// Ownership
// -----------------------------------------------------------------------------

func (t *SSHTarget) HasUser(ctx context.Context, user string) bool {
	_, err := t.resolveUser(ctx, user)
	return err == nil
}

func (t *SSHTarget) HasGroup(ctx context.Context, group string) bool {
	_, err := t.resolveGroup(ctx, group)
	return err == nil
}

func (t *SSHTarget) GetOwner(ctx context.Context, path string) (target.Owner, error) {
	info, err := t.sftp.Stat(path)
	if err != nil {
		if isPermission(err) && t.Escalate != "" {
			return t.escalatedGetOwner(ctx, path)
		}
		return target.Owner{}, normalizeError(err)
	}

	// SFTP returns *sftp.FileStat which has Uid/Gid
	stat, ok := info.Sys().(*sftp.FileStat)
	if !ok {
		panic(errs.BUG("expected *sftp.FileStat, got %T", info.Sys()))
	}

	return target.Owner{
		User:  t.resolveUID(ctx, int(stat.UID)),
		Group: t.resolveGID(ctx, int(stat.GID)),
	}, nil
}

func (t *SSHTarget) Chown(ctx context.Context, path string, owner target.Owner) error {
	// SFTP Chown requires numeric UID/GID
	uid, err := t.resolveUser(ctx, owner.User)
	if err != nil {
		return err
	}
	gid, err := t.resolveGroup(ctx, owner.Group)
	if err != nil {
		return err
	}
	err = t.sftp.Chown(path, uid, gid)
	if isPermission(err) {
		if t.Escalate != "" {
			return t.escalatedChown(ctx, path, owner)
		}
		if !t.IsRoot {
			return target.NoEscalationError{Op: "chown", Path: path}
		}
	}
	return normalizeError(err)
}

// User and group resolution
// -----------------------------------------------------------------------------

func (t *SSHTarget) resolveUser(ctx context.Context, user string) (int, error) {
	if uid, err := strconv.Atoi(user); err == nil {
		return uid, nil
	}

	result, err := t.RunCommand(ctx, fmt.Sprintf("id -u %s", target.ShellQuote(user)))
	if err != nil {
		return 0, err
	}
	if result.ExitCode == 127 {
		return 0, target.ErrCommandNotFound
	}
	if result.ExitCode != 0 {
		return 0, target.ErrUnknownUser
	}

	uid, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
	if err != nil {
		return 0, target.ErrUnknownUser
	}
	return uid, nil
}

func (t *SSHTarget) resolveGroup(ctx context.Context, group string) (int, error) {
	if gid, err := strconv.Atoi(group); err == nil {
		return gid, nil
	}

	result, err := t.RunCommand(ctx, fmt.Sprintf("getent group %s", target.ShellQuote(group)))
	if err != nil {
		return 0, err
	}
	if result.ExitCode == 127 {
		return 0, target.ErrCommandNotFound
	}
	if result.ExitCode != 0 {
		return 0, target.ErrUnknownGroup
	}

	parts := strings.Split(strings.TrimSpace(result.Stdout), ":")
	if len(parts) < 3 {
		return 0, target.ErrUnknownGroup
	}

	gid, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, target.ErrUnknownGroup
	}
	return gid, nil
}

func (t *SSHTarget) resolveUID(ctx context.Context, uid int) string {
	result, err := t.RunCommand(ctx, fmt.Sprintf("getent passwd %d", uid))
	if err != nil || result.ExitCode != 0 {
		return fmt.Sprintf("%d", uid)
	}
	parts := strings.Split(strings.TrimSpace(result.Stdout), ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return fmt.Sprintf("%d", uid)
}

func (t *SSHTarget) resolveGID(ctx context.Context, gid int) string {
	result, err := t.RunCommand(ctx, fmt.Sprintf("getent group %d", gid))
	if err != nil || result.ExitCode != 0 {
		return fmt.Sprintf("%d", gid)
	}
	parts := strings.Split(strings.TrimSpace(result.Stdout), ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return fmt.Sprintf("%d", gid)
}

// bare-error: sentinel for SSH session errors, wrapped via errs.WrapErrf
var errSession = errs.New("ssh session")

func (t *SSHTarget) RunCommand(ctx context.Context, cmd string) (target.CommandResult, error) {
	sess, err := t.pool.acquire(ctx)
	if err != nil {
		return target.CommandResult{}, errs.WrapErrf(errSession, "%v", err)
	}
	defer t.pool.release(sess)

	t.pool.commandsRun.Add(1)
	return sess.run(ctx, cmd)
}

// Escalated fallback methods
// -----------------------------------------------------------------------------

func (t *SSHTarget) escalatedStat(
	ctx context.Context,
	path string,
	followSymlinks bool,
) (fs.FileInfo, error) {
	return escalate.Stat(ctx, t, t.OSInfo.Platform, t.Escalate, path, followSymlinks)
}

func (t *SSHTarget) escalatedGetOwner(
	ctx context.Context,
	path string,
) (target.Owner, error) {
	return escalate.GetOwner(ctx, t, t.OSInfo.Platform, t.Escalate, path)
}

func (t *SSHTarget) escalatedReadFile(ctx context.Context, path string) ([]byte, error) {
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

func (t *SSHTarget) escalatedWriteFile(ctx context.Context, path string, data []byte) error {
	tmp, err := t.writeTempFile(data)
	if err != nil {
		return target.StagingError{Path: path, Err: err}
	}
	defer func() { _ = t.sftp.Remove(tmp) }()

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

func (t *SSHTarget) escalatedRemove(ctx context.Context, path string) error {
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

func (t *SSHTarget) escalatedChmod(ctx context.Context, path string, mode fs.FileMode) error {
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

func (t *SSHTarget) escalatedChown(ctx context.Context, path string, owner target.Owner) error {
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

func (t *SSHTarget) escalatedSymlink(ctx context.Context, tgt, link string) error {
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

func (t *SSHTarget) escalatedMkdir(ctx context.Context, path string, mode fs.FileMode) error {
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

func (t *SSHTarget) writeTempFile(data []byte) (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	tmp := "/tmp/.scampi-" + hex.EncodeToString(buf[:])

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

func isPermission(err error) bool {
	if os.IsPermission(err) {
		return true
	}
	var status *sftp.StatusError
	return errors.As(err, &status) &&
		status.FxCode() == sftp.ErrSSHFxPermissionDenied
}

func isNotExist(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	var status *sftp.StatusError
	return errors.As(err, &status) &&
		status.FxCode() == sftp.ErrSSHFxNoSuchFile
}

func normalizeError(err error) error {
	switch {
	case isNotExist(err):
		return target.ErrNotExist
	case isPermission(err):
		return target.ErrPermission
	}
	return err
}
