// SPDX-License-Identifier: GPL-3.0-only

package local

import (
	"context"
	"os"

	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/posix"
)

func (t POSIXTarget) HasRepoKey(_ context.Context, name string) (bool, error) {
	path := t.RepoKeyPath(name)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (t POSIXTarget) InstallRepoKey(ctx context.Context, cfg target.RepoConfig) error {
	return t.WriteFile(ctx, cfg.KeyPath, cfg.KeyData)
}

func (t POSIXTarget) HasRepo(_ context.Context, name string) (bool, error) {
	path := t.RepoConfigPath(name)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (t POSIXTarget) WriteRepoConfig(ctx context.Context, cfg target.RepoConfig) error {
	content := posix.BuildRepoContent(t.PkgBackend.Kind, cfg)
	return t.WriteFile(ctx, cfg.ConfigPath, []byte(content))
}

func (t POSIXTarget) RemoveRepo(ctx context.Context, name string) error {
	keyPath := t.RepoKeyPath(name)
	cfgPath := t.RepoConfigPath(name)
	_ = t.Remove(ctx, keyPath)
	return t.Remove(ctx, cfgPath)
}
