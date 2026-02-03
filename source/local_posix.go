package source

import (
	"context"
	"os"
)

type LocalPosixSource struct{}

func (LocalPosixSource) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (LocalPosixSource) WriteFile(_ context.Context, path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func (LocalPosixSource) EnsureDir(_ context.Context, path string) error {
	return os.MkdirAll(path, 0o755)
}

func (LocalPosixSource) Stat(_ context.Context, path string) (FileMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FileMeta{Exists: false}, nil
		}
		return FileMeta{}, err
	}

	return FileMeta{
		Exists:   true,
		IsDir:    info.IsDir(),
		Size:     info.Size(),
		Modified: info.ModTime(),
	}, nil
}

func (LocalPosixSource) LookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}
