// SPDX-License-Identifier: GPL-3.0-only

package ssh

import (
	"context"

	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/internal/target/posix"
)

func (t *SSHTarget) HasRepoKey(_ context.Context, name string) (bool, error) {
	path := t.RepoKeyPath(name)
	_, err := t.sftp.Stat(path)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}
		return false, normalizeError(err)
	}
	return true, nil
}

func (t *SSHTarget) InstallRepoKey(ctx context.Context, cfg target.RepoConfig) error {
	return t.WriteFile(ctx, cfg.KeyPath, cfg.KeyData)
}

func (t *SSHTarget) HasRepo(_ context.Context, name string) (bool, error) {
	path := t.RepoConfigPath(name)
	_, err := t.sftp.Stat(path)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}
		return false, normalizeError(err)
	}
	return true, nil
}

func (t *SSHTarget) WriteRepoConfig(ctx context.Context, cfg target.RepoConfig) error {
	content := posix.BuildRepoContent(t.PkgBackend.Kind, cfg)
	return t.WriteFile(ctx, cfg.ConfigPath, []byte(content))
}

func (t *SSHTarget) RemoveRepo(ctx context.Context, name string) error {
	keyPath := t.RepoKeyPath(name)
	cfgPath := t.RepoConfigPath(name)
	_ = t.Remove(ctx, keyPath)
	return t.Remove(ctx, cfgPath)
}
