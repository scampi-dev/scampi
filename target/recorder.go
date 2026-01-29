package target

import (
	"context"
	"io/fs"

	"godoit.dev/doit/capability"
)

type Recorder struct {
	Inner Target

	Reads  []string
	Writes []string
	Stats  []string
	Chmods []string
	Chowns []string
}

func (r *Recorder) ReadFile(ctx context.Context, path string) ([]byte, error) {
	r.Reads = append(r.Reads, path)
	return r.Inner.ReadFile(ctx, path)
}

func (r *Recorder) WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error {
	r.Writes = append(r.Writes, path)
	return r.Inner.WriteFile(ctx, path, data, perm)
}

func (r *Recorder) Stat(ctx context.Context, path string) (fs.FileInfo, error) {
	r.Stats = append(r.Stats, path)
	return r.Inner.Stat(ctx, path)
}

func (r *Recorder) Chmod(ctx context.Context, path string, mode fs.FileMode) error {
	r.Chmods = append(r.Chmods, path)
	return r.Inner.Chmod(ctx, path, mode)
}

func (r *Recorder) Chown(ctx context.Context, path string, owner Owner) error {
	r.Chowns = append(r.Chowns, path)
	return r.Inner.Chown(ctx, path, owner)
}

func (r *Recorder) HasUser(ctx context.Context, user string) bool {
	return r.Inner.HasUser(ctx, user)
}

func (r *Recorder) HasGroup(ctx context.Context, group string) bool {
	return r.Inner.HasGroup(ctx, group)
}

func (r *Recorder) GetOwner(ctx context.Context, path string) (Owner, error) {
	return r.Inner.GetOwner(ctx, path)
}

func (r *Recorder) Lstat(ctx context.Context, path string) (fs.FileInfo, error) {
	return r.Inner.Lstat(ctx, path)
}

func (r *Recorder) Readlink(ctx context.Context, path string) (string, error) {
	return r.Inner.Readlink(ctx, path)
}

func (r *Recorder) Symlink(ctx context.Context, target, link string) error {
	return r.Inner.Symlink(ctx, target, link)
}

func (r *Recorder) Remove(ctx context.Context, path string) error {
	return r.Inner.Remove(ctx, path)
}

func (r *Recorder) Capabilities() capability.Capability {
	return r.Inner.Capabilities()
}
