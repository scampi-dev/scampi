package target

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"godoit.dev/doit/errs"
)

type LocalPosixTarget struct{}

func (LocalPosixTarget) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (LocalPosixTarget) WriteFile(_ context.Context, path string, data []byte, mode fs.FileMode) error {
	return os.WriteFile(path, data, mode)
}

func (LocalPosixTarget) Stat(_ context.Context, path string) (fs.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.WrapErrf(ErrNotExist, "%q", path)
		}

		return nil, err
	}

	return info, nil
}

func (LocalPosixTarget) Lstat(_ context.Context, path string) (fs.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.WrapErrf(ErrNotExist, "%q", path)
		}

		return nil, err
	}

	return info, nil
}

func (LocalPosixTarget) Readlink(_ context.Context, path string) (string, error) {
	return os.Readlink(path)
}

func (LocalPosixTarget) Symlink(_ context.Context, target, link string) error {
	return os.Symlink(target, link)
}

func (LocalPosixTarget) Remove(_ context.Context, path string) error {
	return os.Remove(path)
}

func (LocalPosixTarget) Chown(_ context.Context, path string, owner Owner) error {
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

func (LocalPosixTarget) Chmod(_ context.Context, path string, mode fs.FileMode) error {
	return os.Chmod(path, mode)
}

func (LocalPosixTarget) HasUser(_ context.Context, user string) bool {
	_, err := lookupUser(user)
	return err == nil
}

func (LocalPosixTarget) HasGroup(_ context.Context, group string) bool {
	_, err := lookupGroup(group)
	return err == nil
}

func (LocalPosixTarget) GetOwner(_ context.Context, path string) (Owner, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Owner{}, errs.WrapErrf(ErrNotExist, "%q", path)
		}

		return Owner{}, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Owner{}, errs.BUG("expected %T got %T", &syscall.Stat_t{}, info.Sys())
	}

	usr, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
	if err != nil {
		return Owner{}, err
	}
	grp, err := user.LookupGroupId(strconv.FormatUint(uint64(stat.Gid), 10))
	if err != nil {
		return Owner{}, err
	}

	return Owner{User: usr.Name, Group: grp.Name}, nil
}

func lookupUser(u string) (*user.User, error) {
	if id, ok := isLikelyID(u); ok {
		usr, err := user.LookupId(u)
		if errors.Is(err, user.UnknownUserIdError(id)) {
			return nil, errs.WrapErrf(ErrUnknownUser, "%q", u)
		}
		return usr, err
	}
	usr, err := user.Lookup(u)

	if errors.Is(err, user.UnknownUserError(u)) {
		return nil, errs.WrapErrf(ErrUnknownUser, "%q", u)
	}
	return usr, err
}

func lookupGroup(g string) (*user.Group, error) {
	if _, ok := isLikelyID(g); ok {
		grp, err := user.LookupGroupId(g)
		if errors.Is(err, user.UnknownGroupIdError(g)) {
			return nil, errs.WrapErrf(ErrUnknownGroup, "%q", g)
		}
		return grp, err
	}
	grp, err := user.LookupGroup(g)
	if errors.Is(err, user.UnknownGroupError(g)) {
		return nil, errs.WrapErrf(ErrUnknownGroup, "%q", g)
	}
	return grp, err
}

func isLikelyID(s string) (int, bool) {
	id, err := strconv.Atoi(s)
	return id, err == nil
}
