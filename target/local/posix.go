// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/pkgmgr"
	"scampi.dev/scampi/target/svcmgr"
)

type POSIXTarget struct {
	pkgBackend *pkgmgr.Backend
	svcBackend svcmgr.Backend
	escalate   string // "sudo", "doas", or "" (none)
	isRoot     bool
}

func (t POSIXTarget) ReadFile(ctx context.Context, path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsPermission(err) {
		if t.escalate != "" {
			return t.escalatedReadFile(ctx, path)
		}
		if !t.isRoot {
			return nil, target.NoEscalationError{Op: "read", Path: path}
		}
	}
	return data, err
}

func (t POSIXTarget) WriteFile(ctx context.Context, path string, data []byte) error {
	err := os.WriteFile(path, data, 0o644)
	if os.IsPermission(err) {
		if t.escalate != "" {
			return t.escalatedWriteFile(ctx, path, data)
		}
		if !t.isRoot {
			return target.NoEscalationError{Op: "write", Path: path}
		}
	}
	return err
}

func (POSIXTarget) Stat(_ context.Context, path string) (fs.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}

		return nil, err
	}

	return info, nil
}

func (POSIXTarget) Lstat(_ context.Context, path string) (fs.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}

		return nil, err
	}

	return info, nil
}

func (POSIXTarget) Readlink(_ context.Context, path string) (string, error) {
	return os.Readlink(path)
}

func (t POSIXTarget) Symlink(ctx context.Context, tgt, link string) error {
	err := os.Symlink(tgt, link)
	if os.IsPermission(err) {
		if t.escalate != "" {
			return t.escalatedSymlink(ctx, tgt, link)
		}
		if !t.isRoot {
			return target.NoEscalationError{Op: "symlink", Path: link}
		}
	}
	return err
}

func (t POSIXTarget) Remove(ctx context.Context, path string) error {
	err := os.Remove(path)
	if os.IsPermission(err) {
		if t.escalate != "" {
			return t.escalatedRemove(ctx, path)
		}
		if !t.isRoot {
			return target.NoEscalationError{Op: "remove", Path: path}
		}
	}
	return err
}

func (t POSIXTarget) Mkdir(ctx context.Context, path string, mode fs.FileMode) error {
	err := os.MkdirAll(path, mode)
	if os.IsPermission(err) {
		if t.escalate != "" {
			return t.escalatedMkdir(ctx, path, mode)
		}
		if !t.isRoot {
			return target.NoEscalationError{Op: "mkdir", Path: path}
		}
	}
	return err
}

func (t POSIXTarget) Chown(ctx context.Context, path string, owner target.Owner) error {
	usr, err := lookupUser(owner.User)
	if err != nil {
		return err
	}
	grp, err := lookupGroup(owner.Group)
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(grp.Gid)
	if err != nil {
		return err
	}

	err = os.Chown(path, uid, gid)
	if os.IsPermission(err) {
		if t.escalate != "" {
			return t.escalatedChown(ctx, path, owner)
		}
		if !t.isRoot {
			return target.NoEscalationError{Op: "chown", Path: path}
		}
	}
	return err
}

func (t POSIXTarget) Chmod(ctx context.Context, path string, mode fs.FileMode) error {
	err := os.Chmod(path, mode)
	if os.IsPermission(err) {
		if t.escalate != "" {
			return t.escalatedChmod(ctx, path, mode)
		}
		if !t.isRoot {
			return target.NoEscalationError{Op: "chmod", Path: path}
		}
	}
	return err
}

func (POSIXTarget) HasUser(_ context.Context, user string) bool {
	_, err := lookupUser(user)
	return err == nil
}

func (POSIXTarget) HasGroup(_ context.Context, group string) bool {
	_, err := lookupGroup(group)
	return err == nil
}

func (POSIXTarget) GetOwner(_ context.Context, path string) (target.Owner, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return target.Owner{}, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}

		return target.Owner{}, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return target.Owner{}, errs.BUG("expected %T got %T", &syscall.Stat_t{}, info.Sys())
	}

	usr, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
	if err != nil {
		return target.Owner{}, err
	}
	grp, err := user.LookupGroupId(strconv.FormatUint(uint64(stat.Gid), 10))
	if err != nil {
		return target.Owner{}, err
	}

	return target.Owner{User: usr.Name, Group: grp.Name}, nil
}

func (POSIXTarget) RunCommand(ctx context.Context, cmd string) (target.CommandResult, error) {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return target.CommandResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: exitErr.ExitCode(),
			}, nil
		}
		return target.CommandResult{}, err
	}
	return target.CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}, nil
}

func (t POSIXTarget) Capabilities() capability.Capability {
	caps := capability.POSIX
	if t.pkgBackend != nil {
		caps |= capability.Pkg
		if t.pkgBackend.SupportsUpgrade() {
			caps |= capability.PkgUpdate
		}
	}
	if t.svcBackend != nil {
		caps |= capability.Service
	}
	return caps
}

// Escalated fallback methods
// -----------------------------------------------------------------------------

func (t POSIXTarget) escalatedReadFile(ctx context.Context, path string) ([]byte, error) {
	result, err := t.RunCommand(ctx, t.escalate+" cat "+shellQuote(path))
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

	result, err := t.RunCommand(ctx, t.escalate+" cp "+shellQuote(tmp)+" "+shellQuote(path))
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

func (t POSIXTarget) escalatedRemove(ctx context.Context, path string) error {
	result, err := t.RunCommand(ctx, t.escalate+" rm "+shellQuote(path))
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

func (t POSIXTarget) escalatedChmod(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := t.escalate + " chmod " + octal + " " + shellQuote(path)
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

func (t POSIXTarget) escalatedChown(ctx context.Context, path string, owner target.Owner) error {
	cmd := t.escalate + " chown " +
		shellQuote(owner.User) + ":" + shellQuote(owner.Group) +
		" " + shellQuote(path)
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

func (t POSIXTarget) escalatedSymlink(ctx context.Context, tgt, link string) error {
	cmd := t.escalate + " ln -sfn " +
		shellQuote(tgt) + " " + shellQuote(link)
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

func (t POSIXTarget) escalatedMkdir(ctx context.Context, path string, mode fs.FileMode) error {
	octal := fmt.Sprintf("%04o", mode.Perm())
	cmd := t.escalate + " mkdir -p " + shellQuote(path) +
		" && " + t.escalate + " chmod " + octal + " " + shellQuote(path)
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

func lookupUser(u string) (*user.User, error) {
	if id, ok := isLikelyID(u); ok {
		usr, err := user.LookupId(u)
		if errors.Is(err, user.UnknownUserIdError(id)) {
			return nil, errs.WrapErrf(target.ErrUnknownUser, "%q", u)
		}
		return usr, err
	}
	usr, err := user.Lookup(u)

	if errors.Is(err, user.UnknownUserError(u)) {
		return nil, errs.WrapErrf(target.ErrUnknownUser, "%q", u)
	}
	return usr, err
}

func lookupGroup(g string) (*user.Group, error) {
	if _, ok := isLikelyID(g); ok {
		grp, err := user.LookupGroupId(g)
		if errors.Is(err, user.UnknownGroupIdError(g)) {
			return nil, errs.WrapErrf(target.ErrUnknownGroup, "%q", g)
		}
		return grp, err
	}
	grp, err := user.LookupGroup(g)
	if errors.Is(err, user.UnknownGroupError(g)) {
		return nil, errs.WrapErrf(target.ErrUnknownGroup, "%q", g)
	}
	return grp, err
}

func isLikelyID(s string) (int, bool) {
	id, err := strconv.Atoi(s)
	return id, err == nil
}
