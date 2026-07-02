// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"strconv"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/posix"
)

type POSIXTarget struct {
	posix.Base
}

func (t POSIXTarget) ReadFile(ctx context.Context, path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsPermission(err) {
		if t.Escalate != "" {
			return t.escalatedReadFile(ctx, path)
		}
		if !t.IsRoot {
			return nil, t.NoEscalation("read", path)
		}
	}
	return data, err
}

func (t POSIXTarget) ReadDir(_ context.Context, path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

func (t POSIXTarget) WriteFile(ctx context.Context, path string, data []byte) error {
	err := os.WriteFile(path, data, 0o644)
	if os.IsPermission(err) {
		if t.Escalate != "" {
			return t.escalatedWriteFile(ctx, path, data)
		}
		if !t.IsRoot {
			return t.NoEscalation("write", path)
		}
	}
	return err
}

func (POSIXTarget) Readlink(_ context.Context, path string) (string, error) {
	return os.Readlink(path)
}

func (t POSIXTarget) Symlink(ctx context.Context, tgt, link string) error {
	err := os.Symlink(tgt, link)
	if os.IsPermission(err) {
		if t.Escalate != "" {
			return t.escalatedSymlink(ctx, tgt, link)
		}
		if !t.IsRoot {
			return t.NoEscalation("symlink", link)
		}
	}
	return err
}

func (t POSIXTarget) Remove(ctx context.Context, path string) error {
	err := os.Remove(path)
	if os.IsPermission(err) {
		if t.Escalate != "" {
			return t.escalatedRemove(ctx, path)
		}
		if !t.IsRoot {
			return t.NoEscalation("remove", path)
		}
	}
	return err
}

func (t POSIXTarget) Mkdir(ctx context.Context, path string, mode fs.FileMode) error {
	err := os.MkdirAll(path, mode)
	if os.IsPermission(err) {
		if t.Escalate != "" {
			return t.escalatedMkdir(ctx, path, mode)
		}
		if !t.IsRoot {
			return t.NoEscalation("mkdir", path)
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
		if t.Escalate != "" {
			return t.escalatedChown(ctx, path, owner)
		}
		if !t.IsRoot {
			return t.NoEscalation("chown", path)
		}
	}
	return err
}

func (t POSIXTarget) Chmod(ctx context.Context, path string, mode fs.FileMode) error {
	err := os.Chmod(path, mode)
	if os.IsPermission(err) {
		if t.Escalate != "" {
			return t.escalatedChmod(ctx, path, mode)
		}
		if !t.IsRoot {
			return t.NoEscalation("chmod", path)
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

func (POSIXTarget) RunCommand(ctx context.Context, cmd string) (target.CommandResult, error) {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
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
