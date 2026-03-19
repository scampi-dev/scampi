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

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/escalate"
	"scampi.dev/scampi/target/pkgmgr"
	"scampi.dev/scampi/target/svcmgr"
)

type SSHTarget struct {
	config     *Config
	client     *ssh.Client
	sftp       *sftp.Client
	closeAgent func() error
	osInfo     pkgmgr.OSInfo
	pkgBackend *pkgmgr.Backend
	svcBackend svcmgr.Backend
	escalate   string // "sudo", "doas", or "" (none)
	isRoot     bool
}

func (t *SSHTarget) Capabilities() capability.Capability {
	caps := capability.POSIX // includes User | Group
	if t.pkgBackend != nil {
		caps |= capability.Pkg
		if t.pkgBackend.SupportsUpgrade() {
			caps |= capability.PkgUpdate
		}
		if t.pkgBackend.SupportsRepoSetup() {
			caps |= capability.PkgRepo
		}
	}
	if t.svcBackend != nil {
		caps |= capability.Service
	}
	return caps
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
			if t.escalate != "" {
				return t.escalatedReadFile(ctx, path)
			}
			if !t.isRoot {
				return nil, target.NoEscalationError{Op: "read", Path: path}
			}
		}
		return nil, normalizeError(err)
	}
	defer func() { _ = f.Close() }()

	res, err := io.ReadAll(f)
	return res, normalizeError(err)
}

func (t *SSHTarget) WriteFile(ctx context.Context, path string, data []byte) error {
	f, err := t.sftp.Create(path)
	if err != nil {
		if isPermission(err) {
			if t.escalate != "" {
				return t.escalatedWriteFile(ctx, path, data)
			}
			if !t.isRoot {
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
		if isPermission(err) && t.escalate != "" {
			return escalate.GNUStat(ctx, t, t.escalate, path, true)
		}
		return nil, normalizeError(err)
	}
	return info, nil
}

func (t *SSHTarget) Remove(ctx context.Context, path string) error {
	err := t.sftp.Remove(path)
	if isPermission(err) {
		if t.escalate != "" {
			return t.escalatedRemove(ctx, path)
		}
		if !t.isRoot {
			return target.NoEscalationError{Op: "remove", Path: path}
		}
	}
	return normalizeError(err)
}

func (t *SSHTarget) Mkdir(ctx context.Context, path string, mode fs.FileMode) error {
	err := t.sftp.MkdirAll(path)
	if err != nil {
		if isPermission(err) {
			if t.escalate != "" {
				return t.escalatedMkdir(ctx, path, mode)
			}
			if !t.isRoot {
				return target.NoEscalationError{Op: "mkdir", Path: path}
			}
		}
		return normalizeError(err)
	}
	if err := t.sftp.Chmod(path, mode); err != nil {
		if isPermission(err) {
			if t.escalate != "" {
				return t.escalatedMkdir(ctx, path, mode)
			}
			if !t.isRoot {
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
		if t.escalate != "" {
			return t.escalatedChmod(ctx, path, mode)
		}
		if !t.isRoot {
			return target.NoEscalationError{Op: "chmod", Path: path}
		}
	}
	return normalizeError(err)
}

func (t *SSHTarget) ChmodRecursive(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := "chmod -R " + octal + " " + target.ShellQuote(path)
	if t.escalate != "" {
		cmd = t.escalate + " " + cmd
	}
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.escalate, Op: "chmod -R", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

// Symlinks
// -----------------------------------------------------------------------------

func (t *SSHTarget) Lstat(ctx context.Context, path string) (fs.FileInfo, error) {
	info, err := t.sftp.Lstat(path)
	if err != nil {
		if isPermission(err) && t.escalate != "" {
			return escalate.GNUStat(ctx, t, t.escalate, path, false)
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
		if t.escalate != "" {
			return t.escalatedSymlink(ctx, tgt, link)
		}
		if !t.isRoot {
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
		if isPermission(err) && t.escalate != "" {
			return escalate.GNUGetOwner(ctx, t, t.escalate, path)
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
		if t.escalate != "" {
			return t.escalatedChown(ctx, path, owner)
		}
		if !t.isRoot {
			return target.NoEscalationError{Op: "chown", Path: path}
		}
	}
	return normalizeError(err)
}

func (t *SSHTarget) ChownRecursive(ctx context.Context, path string, owner target.Owner) error {
	cmd := "chown -R " +
		target.ShellQuote(owner.User) + ":" + target.ShellQuote(owner.Group) +
		" " + target.ShellQuote(path)
	if t.escalate != "" {
		cmd = t.escalate + " " + cmd
	}
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.escalate, Op: "chown -R", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

// User and group resolution
// -----------------------------------------------------------------------------

func (t *SSHTarget) resolveUser(ctx context.Context, user string) (int, error) {
	// Try numeric first
	if uid, err := strconv.Atoi(user); err == nil {
		return uid, nil
	}

	// Use `id` command
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
	// Try numeric first
	if gid, err := strconv.Atoi(group); err == nil {
		return gid, nil
	}

	// Use `getent` command
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

	// getent output: "groupname:x:gid:members"
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
		return fmt.Sprintf("%d", uid) // Fall back to numeric
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
		return fmt.Sprintf("%d", gid) // Fall back to numeric
	}
	parts := strings.Split(strings.TrimSpace(result.Stdout), ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return fmt.Sprintf("%d", gid)
}

var errSession = errors.New("ssh session")

func (t *SSHTarget) RunCommand(_ context.Context, cmd string) (target.CommandResult, error) {
	session, err := t.client.NewSession()
	if err != nil {
		return target.CommandResult{}, errs.WrapErrf(errSession, "%v", err)
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
		return target.CommandResult{}, errs.WrapErrf(errSession, "%v", err)
	}

	return target.CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}, nil
}

func (t *SSHTarget) RunPrivileged(ctx context.Context, cmd string) (target.CommandResult, error) {
	if t.escalate != "" {
		cmd = t.escalate + " " + cmd
	}
	return t.RunCommand(ctx, cmd)
}

// Escalated fallback methods
// -----------------------------------------------------------------------------

func (t *SSHTarget) escalatedReadFile(ctx context.Context, path string) ([]byte, error) {
	result, err := t.RunCommand(ctx, t.escalate+" cat "+target.ShellQuote(path))
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, target.EscalationError{
			Tool: t.escalate, Op: "cat", Path: path,
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

	result, err := t.RunCommand(ctx, t.escalate+" cp "+target.ShellQuote(tmp)+" "+target.ShellQuote(path))
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.escalate, Op: "cp", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t *SSHTarget) escalatedRemove(ctx context.Context, path string) error {
	result, err := t.RunCommand(ctx, t.escalate+" rm "+target.ShellQuote(path))
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.escalate, Op: "rm", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t *SSHTarget) escalatedChmod(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := t.escalate + " chmod " + octal + " " + target.ShellQuote(path)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.escalate, Op: "chmod", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t *SSHTarget) escalatedChown(ctx context.Context, path string, owner target.Owner) error {
	cmd := t.escalate + " chown " +
		target.ShellQuote(owner.User) + ":" + target.ShellQuote(owner.Group) +
		" " + target.ShellQuote(path)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.escalate, Op: "chown", Path: path,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t *SSHTarget) escalatedSymlink(ctx context.Context, tgt, link string) error {
	cmd := t.escalate + " ln -sfn " +
		target.ShellQuote(tgt) + " " + target.ShellQuote(link)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.escalate, Op: "ln", Path: link,
			Stderr: result.Stderr, ExitCode: result.ExitCode,
		}
	}
	return nil
}

func (t *SSHTarget) escalatedMkdir(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := t.escalate + " mkdir -p " + target.ShellQuote(path) +
		" && " + t.escalate + " chmod " + octal + " " + target.ShellQuote(path)
	result, err := t.RunCommand(ctx, cmd)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return target.EscalationError{
			Tool: t.escalate, Op: "mkdir", Path: path,
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
