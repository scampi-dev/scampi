// SPDX-License-Identifier: GPL-3.0-only

package secret

import (
	"path/filepath"
	"strings"

	"filippo.io/age"
	"scampi.dev/scampi/internal/errs"
)

const (
	envAgeKey     = "SCAMPI_AGE_KEY"
	envAgeKeyFile = "SCAMPI_AGE_KEY_FILE"
)

// ResolveIdentities finds age identities using a three-step chain:
//  1. $SCAMPI_AGE_KEY — raw private key string
//  2. $SCAMPI_AGE_KEY_FILE — path to a key file
//  3. ~/.config/scampi/age.key — default location
//
// lookupEnv and readFile are injected so the caller decides where
// "the environment" and "the filesystem" come from (source abstraction
// in eval, real OS in CLI).
func ResolveIdentities(
	lookupEnv func(string) (string, bool),
	readFile func(string) ([]byte, error),
) ([]age.Identity, error) {
	// 1. Raw key from environment
	if raw, ok := lookupEnv(envAgeKey); ok && raw != "" {
		id, err := age.ParseX25519Identity(strings.TrimSpace(raw))
		if err != nil {
			// bare-error: identity file error, wrapped by SecretBackendError before reaching engine
			return nil, errs.Errorf("parsing %s: %w", envAgeKey, err)
		}
		return []age.Identity{id}, nil
	}

	// 2. Key file path from environment
	if path, ok := lookupEnv(envAgeKeyFile); ok && path != "" {
		ids, err := parseKeyFile(readFile, path)
		if err != nil {
			// bare-error: identity file error, wrapped by SecretBackendError before reaching engine
			return nil, errs.Errorf("reading %s (%s): %w", envAgeKeyFile, path, err)
		}
		return ids, nil
	}

	// 3. Default key file
	defaultPath := DefaultAgeKeyPath(configDir(lookupEnv))
	ids, err := parseKeyFile(readFile, defaultPath)
	if err == nil {
		return ids, nil
	}

	// bare-error: identity file error, wrapped by SecretBackendError before reaching engine
	return nil, errs.Errorf(
		"no age identity found: set $%s, $%s, or create %s",
		envAgeKey, envAgeKeyFile, defaultPath,
	)
}

// DefaultAgeKeyPath returns the conventional path to the age identity file
// under the given config directory.
func DefaultAgeKeyPath(configDir string) string {
	return filepath.Join(configDir, "scampi", "age.key")
}

// configDir resolves the user config directory from the environment,
// respecting XDG_CONFIG_HOME on all platforms.
func configDir(lookupEnv func(string) (string, bool)) string {
	if xdg, ok := lookupEnv("XDG_CONFIG_HOME"); ok && xdg != "" {
		return xdg
	}
	if home, ok := lookupEnv("HOME"); ok && home != "" {
		return filepath.Join(home, ".config")
	}
	return ".config"
}

func parseKeyFile(readFile func(string) ([]byte, error), path string) ([]age.Identity, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	ids, err := age.ParseIdentities(strings.NewReader(string(data)))
	if err != nil {
		// bare-error: identity file error, wrapped by SecretBackendError before reaching engine
		return nil, errs.Errorf("parsing key file %s: %w", path, err)
	}
	return ids, nil
}
