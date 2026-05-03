// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"bytes"
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
	"time"

	"github.com/cenkalti/backoff/v4"
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
// and shared for the target's lifetime. All RunCommand calls open a
// new SSH session (channel) on that existing client — channels are
// cheap (no TCP, no auth, no kex), so the per-op cost is just a small
// channel-open round-trip plus the actual command time.
//
// The same single-connection model applies to the SFTP subsystem: one
// `*sftp.Client` reuses the SSH connection for all file operations.
//
// On top of the pooled connection, RunCommand goes through two
// independent backpressure mechanisms:
//
//  1. **Local sanity cap**: a bounded slot pool of MaxSessions. This
//     is a hard ceiling on how many concurrent sessions WE will ever
//     try to open against the target, regardless of what the server
//     would allow. It defaults to DefaultMaxSessions (10, matching
//     OpenSSH's sshd_config default) and can be raised per-target if
//     sysadmins have lifted sshd's MaxSessions. The slot pool keeps
//     us from hammering a server we know little about.
//
//  2. **Server backpressure**: if the server still rejects a channel
//     open (because its actual MaxSessions is lower than ours, or
//     transient load), retryNewSession waits with exponential
//     backoff + jitter and tries again. The server is the source of
//     truth; we just listen.
//
// Sessions in `golang.org/x/crypto/ssh` are strictly one-shot —
// after `session.Run` returns, that session is done. So what's
// actually pooled here is the *right to have a session open*, not
// the session object itself. When the slot is released, the next
// caller opens a fresh session on the same TCP connection.
//
// Stats() exposes counters so the pooling invariants can be tested
// and any regression (a stray re-dial, slot leak, unexpected queue
// depth) is visible.

// DefaultMaxSessions is the slot pool size used when Config.MaxSessions
// is unset. Matches OpenSSH's sshd_config default of MaxSessions=10
// — conservative enough to never hammer a stock server.
const DefaultMaxSessions = 10

// sessionOpenBackoff returns a fresh exponential-backoff schedule for
// reopening an SSH session after the server rejected one. Aggressive
// initial wait (10ms) absorbs the SFTP-vs-MaxSessions race; capped at
// 1s per attempt and 5s total before giving up.
func sessionOpenBackoff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 10 * time.Millisecond
	b.MaxInterval = 1 * time.Second
	b.Multiplier = 2.0
	b.RandomizationFactor = 0.5
	b.MaxElapsedTime = 5 * time.Second
	b.Reset()
	return b
}

// SSHStats reports per-target SSH usage counters.
//
//   - DialCount stays at 1 for the target's lifetime.
//   - MaxSessions is the configured slot pool capacity.
//   - SessionsOpened increments once per RunCommand.
//   - SessionsPeakInFlight tracks the high-water mark of concurrent
//     sessions actually open at the same instant.
//   - SessionRetries counts how many times a NewSession was retried
//     after the server rejected it. Non-zero means we hit server-side
//     rate limits — useful diagnostic for tuning MaxSessions.
type SSHStats struct {
	DialCount            int64
	MaxSessions          int64
	SessionsOpened       int64
	SessionsPeakInFlight int64
	SessionRetries       int64
}

type SSHTarget struct {
	posix.Base
	config     *Config
	client     *ssh.Client
	sftp       *sftp.Client
	closeAgent func() error

	// slots is the bounded session pool — a hard sanity cap on
	// concurrent in-flight sessions. Capacity is config.MaxSessions
	// (default DefaultMaxSessions). RunCommand acquires before
	// opening, releases on session close. Acquisition respects ctx.
	slots       chan struct{}
	maxSessions int

	dialCount        atomic.Int64
	sessionsOpened   atomic.Int64
	sessionsInFlight atomic.Int64
	sessionsPeakSeen atomic.Int64
	sessionRetries   atomic.Int64
}

// Stats returns a snapshot of SSH usage counters for this target.
func (t *SSHTarget) Stats() SSHStats {
	return SSHStats{
		DialCount:            t.dialCount.Load(),
		MaxSessions:          int64(t.maxSessions),
		SessionsOpened:       t.sessionsOpened.Load(),
		SessionsPeakInFlight: t.sessionsPeakSeen.Load(),
		SessionRetries:       t.sessionRetries.Load(),
	}
}

// retryNewSession opens a session, retrying with exponential backoff
// and jitter when the server refuses the channel open. Server-side
// rejection is how we discover the actual cap on a connection — we
// don't know it up front, we just listen and back off. Honors ctx
// for cancellation throughout.
func (t *SSHTarget) retryNewSession(ctx context.Context) (*ssh.Session, error) {
	var session *ssh.Session
	op := func() error {
		s, err := t.client.NewSession()
		if err != nil {
			t.sessionRetries.Add(1)
			return err
		}
		session = s
		return nil
	}
	if err := backoff.Retry(op, backoff.WithContext(sessionOpenBackoff(), ctx)); err != nil {
		return nil, err
	}
	// First attempt counts as 0 retries; sessionRetries was bumped on
	// every failure inside op, so the final successful call doesn't
	// over-count.
	return session, nil
}

// acquireSlot blocks until a session slot is free or ctx is cancelled.
// The caller must call releaseSlot when the session is closed.
func (t *SSHTarget) acquireSlot(ctx context.Context) error {
	select {
	case t.slots <- struct{}{}:
		// Track in-flight count for diagnostics.
		now := t.sessionsInFlight.Add(1)
		for {
			peak := t.sessionsPeakSeen.Load()
			if now <= peak || t.sessionsPeakSeen.CompareAndSwap(peak, now) {
				break
			}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *SSHTarget) releaseSlot() {
	t.sessionsInFlight.Add(-1)
	<-t.slots
}

func (t *SSHTarget) Close() {
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
	if err := t.acquireSlot(ctx); err != nil {
		return target.CommandResult{}, errs.WrapErrf(errSession, "%v", err)
	}
	defer t.releaseSlot()

	session, err := t.retryNewSession(ctx)
	if err != nil {
		return target.CommandResult{}, errs.WrapErrf(errSession, "%v", err)
	}
	t.sessionsOpened.Add(1)
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
		return target.CommandResult{}, errs.WrapErrf(errSession, "%v", err)
	}

	return target.CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}, nil
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
