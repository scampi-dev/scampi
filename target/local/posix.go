package local

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"godoit.dev/doit/capability"
	"godoit.dev/doit/errs"
	"godoit.dev/doit/target"
)

type POSIXTarget struct{}

func (POSIXTarget) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (POSIXTarget) WriteFile(_ context.Context, path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
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

func (POSIXTarget) Symlink(_ context.Context, target, link string) error {
	return os.Symlink(target, link)
}

func (POSIXTarget) Remove(_ context.Context, path string) error {
	return os.Remove(path)
}

func (POSIXTarget) Chown(_ context.Context, path string, owner target.Owner) error {
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

	return os.Chown(path, uid, gid)
}

func (POSIXTarget) Chmod(_ context.Context, path string, mode fs.FileMode) error {
	return os.Chmod(path, mode)
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

func (POSIXTarget) Capabilities() capability.Capability {
	return capability.POSIX
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
