// SPDX-License-Identifier: GPL-3.0-only

package secret

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"filippo.io/age"
	"scampi.dev/scampi/internal/errs"
)

const (
	agePrefix = "AGE["
	ageSuffix = "]"
)

// AgeBackend loads secrets from a JSON file where values are individually
// encrypted with age. Keys are plaintext; values are wrapped as AGE[base64(ciphertext)].
type AgeBackend struct {
	secrets map[string]string
}

func NewAgeBackend(data []byte, identities []age.Identity) (*AgeBackend, error) {
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		// bare-error: secret backend error, wrapped by SecretBackendError before reaching engine
		return nil, errs.Errorf("parsing age secrets file: %w", err)
	}

	decrypted := make(map[string]string, len(raw))
	for k, v := range raw {
		plain, err := DecryptValue(v, identities)
		if err != nil {
			// bare-error: secret backend error, wrapped by SecretBackendError before reaching engine
			return nil, errs.Errorf("decrypting key %q: %w", k, err)
		}
		decrypted[k] = plain
	}

	return &AgeBackend{secrets: decrypted}, nil
}

func (a *AgeBackend) Name() string { return "age" }

func (a *AgeBackend) Lookup(key string) (string, bool, error) {
	v, ok := a.secrets[key]
	return v, ok, nil
}

func (a *AgeBackend) Keys() []string { return SortedKeys(a.secrets) }

// EncryptValue encrypts plaintext with the given recipients and returns
// the wrapped AGE[base64(ciphertext)] string.
func EncryptValue(plaintext string, recipients []age.Recipient) (string, error) {
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipients...)
	if err != nil {
		// bare-error: secret backend error, wrapped by SecretBackendError before reaching engine
		return "", errs.Errorf("creating age encryptor: %w", err)
	}
	if _, err := io.WriteString(w, plaintext); err != nil {
		// bare-error: secret backend error, wrapped by SecretBackendError before reaching engine
		return "", errs.Errorf("writing plaintext: %w", err)
	}
	if err := w.Close(); err != nil {
		// bare-error: secret backend error, wrapped by SecretBackendError before reaching engine
		return "", errs.Errorf("finalizing encryption: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return agePrefix + encoded + ageSuffix, nil
}

// DecryptValue unwraps an AGE[base64(ciphertext)] string and decrypts it.
func DecryptValue(wrapped string, identities []age.Identity) (string, error) {
	if !IsAgeEncrypted(wrapped) {
		// bare-error: secret backend error, wrapped by SecretBackendError before reaching engine
		return "", errs.Errorf("value is not age-encrypted (missing %s...%s wrapper)", agePrefix, ageSuffix)
	}

	encoded := wrapped[len(agePrefix) : len(wrapped)-len(ageSuffix)]
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// bare-error: secret backend error, wrapped by SecretBackendError before reaching engine
		return "", errs.Errorf("decoding base64: %w", err)
	}

	r, err := age.Decrypt(bytes.NewReader(ciphertext), identities...)
	if err != nil {
		// bare-error: secret backend error, wrapped by SecretBackendError before reaching engine
		return "", errs.Errorf("age decrypt: %w", err)
	}

	plain, err := io.ReadAll(r)
	if err != nil {
		// bare-error: secret backend error, wrapped by SecretBackendError before reaching engine
		return "", errs.Errorf("reading decrypted data: %w", err)
	}

	return string(plain), nil
}

// IsAgeEncrypted reports whether s has the AGE[...] wrapper.
func IsAgeEncrypted(s string) bool {
	return strings.HasPrefix(s, agePrefix) && strings.HasSuffix(s, ageSuffix)
}
