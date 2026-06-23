// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin || freebsd || netbsd || openbsd

package local

import (
	"context"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"scampi.dev/scampi/internal/errs"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/escalate"
)

func (t POSIXTarget) Stat(ctx context.Context, path string) (fs.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		if os.IsPermission(err) && t.Escalate != "" {
			return escalate.BSDStat(ctx, t, t.Escalate, path, true)
		}
		return nil, err
	}
	return info, nil
}

func (t POSIXTarget) Lstat(ctx context.Context, path string) (fs.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		if os.IsPermission(err) && t.Escalate != "" {
			return escalate.BSDStat(ctx, t, t.Escalate, path, false)
		}
		return nil, err
	}
	return info, nil
}

func (t POSIXTarget) GetOwner(ctx context.Context, path string) (target.Owner, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return target.Owner{}, errs.WrapErrf(target.ErrNotExist, "%q", path)
		}
		if os.IsPermission(err) && t.Escalate != "" {
			return escalate.BSDGetOwner(ctx, t, t.Escalate, path)
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
