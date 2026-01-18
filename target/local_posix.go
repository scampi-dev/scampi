package target

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"godoit.dev/doit/util"
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
			return nil, fmt.Errorf("%w: %q", ErrNotExist, path)
		}

		return nil, err
	}

	return info, nil
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

func (LocalPosixTarget) GetOwner(_ context.Context, path string) (Owner, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Owner{}, fmt.Errorf("%w: %q", ErrNotExist, path)
		}

		return Owner{}, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Owner{}, util.BUG("expected %T got %T", &syscall.Stat_t{}, info.Sys())
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
	if isLikelyID(u) {
		return user.LookupId(u)
	}
	return user.Lookup(u)
}

func lookupGroup(g string) (*user.Group, error) {
	if isLikelyID(g) {
		return user.LookupGroupId(g)
	}
	return user.LookupGroup(g)
}

func isLikelyID(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
