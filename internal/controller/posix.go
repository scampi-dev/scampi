// SPDX-License-Identifier: GPL-3.0-only

package controller

import (
	"context"
	"os"
)

type Posix struct{}

func (Posix) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (Posix) WriteFile(_ context.Context, path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func (Posix) EnsureDir(_ context.Context, path string) error {
	return os.MkdirAll(path, 0o755)
}

func (Posix) Stat(_ context.Context, path string) (FileMeta, error) {
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

func (Posix) LookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}
