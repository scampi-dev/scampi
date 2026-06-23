// SPDX-License-Identifier: GPL-3.0-only

package secret

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"

	"filippo.io/age"
)

func generateTestKeypair(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generating keypair: %v", err)
	}
	return id
}

// Encrypt/Decrypt round-trip
// -----------------------------------------------------------------------------

func TestAge_RoundTrip(t *testing.T) {
	id := generateTestKeypair(t)
	plaintext := "hunter2"

	encrypted, err := EncryptValue(plaintext, []age.Recipient{id.Recipient()})
	if err != nil {
		t.Fatalf("EncryptValue: %v", err)
	}

	if !IsAgeEncrypted(encrypted) {
		t.Fatalf("encrypted value should have AGE[...] wrapper, got %q", encrypted)
	}

	decrypted, err := DecryptValue(encrypted, []age.Identity{id})
	if err != nil {
		t.Fatalf("DecryptValue: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("round-trip mismatch: got %q, want %q", decrypted, plaintext)
	}
}

// IsAgeEncrypted
// -----------------------------------------------------------------------------

func TestIsAgeEncrypted(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"AGE[abc123]", true},
		{"AGE[]", true},
		{"plaintext", false},
		{"AGE[incomplete", false},
		{"incomplete]", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsAgeEncrypted(tt.input); got != tt.want {
			t.Errorf("IsAgeEncrypted(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// NewAgeBackend
// -----------------------------------------------------------------------------

func TestAgeBackend_ValidEncryptedJSON(t *testing.T) {
	id := generateTestKeypair(t)

	enc1, err := EncryptValue("hunter2", []age.Recipient{id.Recipient()})
	if err != nil {
		t.Fatal(err)
	}
	enc2, err := EncryptValue("tok-abc", []age.Recipient{id.Recipient()})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := json.Marshal(map[string]string{
		"db_pass":   enc1,
		"api_token": enc2,
	})

	b, err := NewAgeBackend(data, []age.Identity{id})
	if err != nil {
		t.Fatalf("NewAgeBackend: %v", err)
	}

	if b.Name() != "age" {
		t.Errorf("Name: got %q, want %q", b.Name(), "age")
	}

	val, ok, err := b.Lookup("db_pass")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !ok {
		t.Fatal("Lookup(db_pass): not found")
	}
	if val != "hunter2" {
		t.Errorf("Lookup(db_pass): got %q, want %q", val, "hunter2")
	}

	_, ok, _ = b.Lookup("missing")
	if ok {
		t.Error("Lookup(missing): should not be found")
	}
}

func TestAgeBackend_InvalidBase64(t *testing.T) {
	data, _ := json.Marshal(map[string]string{
		"key": "AGE[not-valid-base64!!!]",
	})

	id := generateTestKeypair(t)
	_, err := NewAgeBackend(data, []age.Identity{id})
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestAgeBackend_WrongIdentity(t *testing.T) {
	encryptor := generateTestKeypair(t)
	wrongKey := generateTestKeypair(t)

	enc, err := EncryptValue("secret", []age.Recipient{encryptor.Recipient()})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := json.Marshal(map[string]string{"key": enc})

	_, err = NewAgeBackend(data, []age.Identity{wrongKey})
	if err == nil {
		t.Fatal("expected error for wrong identity")
	}
}

func TestAgeBackend_InvalidJSON(t *testing.T) {
	id := generateTestKeypair(t)
	_, err := NewAgeBackend([]byte(`not json`), []age.Identity{id})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ResolveIdentities
// -----------------------------------------------------------------------------

func TestResolveIdentities_EnvKey(t *testing.T) {
	id := generateTestKeypair(t)

	lookup := func(key string) (string, bool) {
		if key == envAgeKey {
			return id.String(), true
		}
		return "", false
	}
	readFile := func(string) ([]byte, error) {
		return nil, fmt.Errorf("should not be called")
	}

	ids, err := ResolveIdentities(lookup, readFile)
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(ids))
	}
}

func TestResolveIdentities_EnvKeyFile(t *testing.T) {
	id := generateTestKeypair(t)

	lookup := func(key string) (string, bool) {
		if key == envAgeKeyFile {
			return "/tmp/test.key", true
		}
		return "", false
	}
	readFile := func(path string) ([]byte, error) {
		if path == "/tmp/test.key" {
			return []byte(id.String()), nil
		}
		return nil, fs.ErrNotExist
	}

	ids, err := ResolveIdentities(lookup, readFile)
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(ids))
	}
}

func TestResolveIdentities_DefaultFile(t *testing.T) {
	id := generateTestKeypair(t)
	expectedPath := "/fake/home/.config/scampi/age.key"

	lookup := func(key string) (string, bool) {
		if key == "HOME" {
			return "/fake/home", true
		}
		return "", false
	}
	readFile := func(path string) ([]byte, error) {
		if path == expectedPath {
			return []byte(id.String()), nil
		}
		return nil, fs.ErrNotExist
	}

	ids, err := ResolveIdentities(lookup, readFile)
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(ids))
	}
}

func TestResolveIdentities_NothingAvailable(t *testing.T) {
	lookup := func(string) (string, bool) {
		return "", false
	}
	readFile := func(string) ([]byte, error) {
		return nil, fs.ErrNotExist
	}

	_, err := ResolveIdentities(lookup, readFile)
	if err == nil {
		t.Fatal("expected error when no identity is available")
	}
}
