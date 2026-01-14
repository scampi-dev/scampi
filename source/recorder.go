package source

import "context"

type Recorder struct {
	Inner Source

	Reads  []string
	Writes []string
	Stats  []string
	Mkdirs []string
}

func (r *Recorder) ReadFile(ctx context.Context, path string) ([]byte, error) {
	r.Reads = append(r.Reads, path)
	return r.Inner.ReadFile(ctx, path)
}

func (r *Recorder) WriteFile(ctx context.Context, path string, data []byte) error {
	r.Writes = append(r.Writes, path)
	return r.Inner.WriteFile(ctx, path, data)
}

func (r *Recorder) EnsureDir(ctx context.Context, path string) error {
	r.Mkdirs = append(r.Mkdirs, path)
	return r.Inner.EnsureDir(ctx, path)
}

func (r *Recorder) Stat(ctx context.Context, path string) (FileMeta, error) {
	r.Stats = append(r.Stats, path)
	return r.Inner.Stat(ctx, path)
}
