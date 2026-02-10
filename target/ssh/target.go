package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"godoit.dev/doit/capability"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/target"
	"golang.org/x/crypto/ssh"
)

type SSHTarget struct {
	config     *Config
	client     *ssh.Client
	sftp       *sftp.Client
	closeAgent func() error
}

func (t *SSHTarget) Capabilities() capability.Capability {
	return capability.POSIX
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

func (t *SSHTarget) ReadFile(_ context.Context, path string) ([]byte, error) {
	f, err := t.sftp.Open(path)
	if err != nil {
		return nil, normalizeError(err)
	}
	defer func() { _ = f.Close() }()

	res, err := io.ReadAll(f)
	return res, normalizeError(err)
}

func (t *SSHTarget) WriteFile(_ context.Context, path string, data []byte) error {
	f, err := t.sftp.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = f.Write(data)
	return normalizeError(err)
}

func (t *SSHTarget) Stat(_ context.Context, path string) (fs.FileInfo, error) {
	info, err := t.sftp.Stat(path)
	if err != nil {
		return nil, normalizeError(err)
	}
	return info, nil
}

func (t *SSHTarget) Remove(_ context.Context, path string) error {
	return normalizeError(t.sftp.Remove(path))
}

// FileMode
// -----------------------------------------------------------------------------

func (t *SSHTarget) Chmod(_ context.Context, path string, mode fs.FileMode) error {
	return normalizeError(t.sftp.Chmod(path, mode))
}

// Symlinks
// -----------------------------------------------------------------------------

func (t *SSHTarget) Lstat(_ context.Context, path string) (fs.FileInfo, error) {
	info, err := t.sftp.Lstat(path)
	if err != nil {
		return nil, normalizeError(err)
	}
	return info, nil
}

func (t *SSHTarget) Readlink(_ context.Context, path string) (string, error) {
	res, err := t.sftp.ReadLink(path)
	return res, normalizeError(err)
}

func (t *SSHTarget) Symlink(_ context.Context, target, link string) error {
	return normalizeError(t.sftp.Symlink(target, link))
}

// Ownership
// -----------------------------------------------------------------------------

func (t *SSHTarget) HasUser(_ context.Context, user string) bool {
	_, err := t.resolveUser(user)
	return err == nil
}

func (t *SSHTarget) HasGroup(_ context.Context, group string) bool {
	_, err := t.resolveGroup(group)
	return err == nil
}

func (t *SSHTarget) GetOwner(_ context.Context, path string) (target.Owner, error) {
	info, err := t.sftp.Stat(path)
	if err != nil {
		return target.Owner{}, normalizeError(err)
	}

	// SFTP returns *sftp.FileStat which has Uid/Gid
	stat, ok := info.Sys().(*sftp.FileStat)
	if !ok {
		panic(errs.BUG("expected *sftp.FileStat, got %T", info.Sys()))
	}

	return target.Owner{
		User:  t.resolveUID(int(stat.UID)),
		Group: t.resolveGID(int(stat.GID)),
	}, nil
}

func (t *SSHTarget) Chown(_ context.Context, path string, owner target.Owner) error {
	// SFTP Chown requires numeric UID/GID
	uid, err := t.resolveUser(owner.User)
	if err != nil {
		return err
	}
	gid, err := t.resolveGroup(owner.Group)
	if err != nil {
		return err
	}
	return normalizeError(t.sftp.Chown(path, uid, gid))
}

// User and group resolution
// -----------------------------------------------------------------------------

func (t *SSHTarget) resolveUser(user string) (int, error) {
	// Try numeric first
	if uid, err := strconv.Atoi(user); err == nil {
		return uid, nil
	}

	// Use `id` command
	result, err := t.runCommand(fmt.Sprintf("id -u %s", shellQuote(user)))
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

func (t *SSHTarget) resolveGroup(group string) (int, error) {
	// Try numeric first
	if gid, err := strconv.Atoi(group); err == nil {
		return gid, nil
	}

	// Use `getent` command
	result, err := t.runCommand(fmt.Sprintf("getent group %s", shellQuote(group)))
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

func (t *SSHTarget) resolveUID(uid int) string {
	result, err := t.runCommand(fmt.Sprintf("getent passwd %d", uid))
	if err != nil || result.ExitCode != 0 {
		return fmt.Sprintf("%d", uid) // Fall back to numeric
	}
	parts := strings.Split(strings.TrimSpace(result.Stdout), ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return fmt.Sprintf("%d", uid)
}

func (t *SSHTarget) resolveGID(gid int) string {
	result, err := t.runCommand(fmt.Sprintf("getent group %d", gid))
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

type cmdResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func (t *SSHTarget) runCommand(cmd string) (cmdResult, error) {
	session, err := t.client.NewSession()
	if err != nil {
		return cmdResult{}, errs.WrapErrf(errSession, "%v", err)
	}
	defer func() { _ = session.Close() }()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(cmd)
	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return cmdResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: exitErr.ExitStatus(),
			}, nil
		}
		return cmdResult{}, errs.WrapErrf(errSession, "%v", err)
	}

	return cmdResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func normalizeError(err error) error {
	switch {
	case os.IsNotExist(err):
		return target.ErrNotExist
	case os.IsPermission(err):
		return target.ErrPermission
	}
	return err
}
