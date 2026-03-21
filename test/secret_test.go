// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"filippo.io/age"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/star"
	"scampi.dev/scampi/target"
)

// TestSecret_ResolvesIntoTemplateData verifies that secret() values flow
// through to template rendering.
func TestSecret_ResolvesIntoTemplateData(t *testing.T) {
	cfgStr := `
secrets(backend="file", path="/secrets.json")

target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="secret-template",
            src=inline("pass={{.db_pass}}"),
            dest="/out.txt",
            data={
                "values": {
                    "db_pass": secret("db_pass"),
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/secrets.json"] = []byte(`{"db_pass": "hunter2"}`)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	data, ok := tgt.Files["/out.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	if string(data) != "pass=hunter2" {
		t.Errorf("unexpected content: got %q, want %q", data, "pass=hunter2")
	}
}

// TestSecret_NotFound verifies that a missing secret produces an abort.
func TestSecret_NotFound(t *testing.T) {
	cfgStr := `
secrets(backend="file", path="/secrets.json")

target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="missing-secret",
            src=inline("{{.token}}"),
            dest="/out.txt",
            data={
                "values": {
                    "token": secret("missing_key"),
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)
	src.Files["/secrets.json"] = []byte(`{}`)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}

	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}

	var notFound *star.SecretNotFoundError
	found := false
	for _, cause := range abort.Causes {
		if errors.As(cause, &notFound) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SecretNotFoundError in causes, got: %v", abort.Causes)
	}
}

// TestSecret_WrongArgType verifies secret() rejects non-string keys.
func TestSecret_WrongArgType(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="bad-secret",
            src=inline("{{.x}}"),
            dest="/out.txt",
            data={
                "values": {
                    "x": secret(42),
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for wrong arg type, got nil")
	}

	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}

	var secretErr *star.SecretError
	found := false
	for _, cause := range abort.Causes {
		if errors.As(cause, &secretErr) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SecretError in causes, got: %v", abort.Causes)
	}
}

// TestSecret_TooManyArgs verifies secret() rejects extra arguments.
func TestSecret_TooManyArgs(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="too-many",
            src=inline("{{.x}}"),
            dest="/out.txt",
            data={
                "values": {
                    "x": secret("a", "b"),
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for too many args, got nil")
	}
}

// TestSecret_NoBackend verifies secret() without secrets() gives a clear error.
func TestSecret_NoBackend(t *testing.T) {
	cfgStr := `
target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="no-backend",
            src=inline("{{.x}}"),
            dest="/out.txt",
            data={
                "values": {
                    "x": secret("something"),
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for missing backend, got nil")
	}

	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}

	var cfgErr *star.SecretsConfigError
	found := false
	for _, cause := range abort.Causes {
		if errors.As(cause, &cfgErr) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SecretsConfigError in causes, got: %v", abort.Causes)
	}
}

// secrets() builtin tests
// -----------------------------------------------------------------------------

// TestSecrets_FileBackend verifies secrets(backend="file") configures the backend.
func TestSecrets_FileBackend(t *testing.T) {
	cfgStr := `
secrets(backend="file", path="/my-secrets.json")

target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="explicit-backend",
            src=inline("token={{.api_token}}"),
            dest="/out.txt",
            data={
                "values": {
                    "api_token": secret("api_token"),
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/my-secrets.json"] = []byte(`{"api_token": "tok-abc123"}`)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	data, ok := tgt.Files["/out.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	if string(data) != "token=tok-abc123" {
		t.Errorf("unexpected content: got %q, want %q", data, "token=tok-abc123")
	}
}

// TestSecrets_MissingPath verifies secrets() rejects a missing path argument.
func TestSecrets_MissingPath(t *testing.T) {
	cfgStr := `
secrets(backend="file")

target.local(name="local")
deploy(name="test", targets=["local"], steps=[])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}

// TestSecrets_AgeMissingPath verifies secrets(backend="age") rejects a missing path.
func TestSecrets_AgeMissingPath(t *testing.T) {
	cfgStr := `
secrets(backend="age")

target.local(name="local")
deploy(name="test", targets=["local"], steps=[])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}

// TestSecrets_CalledTwice verifies secrets() rejects a second call.
func TestSecrets_CalledTwice(t *testing.T) {
	cfgStr := `
secrets(backend="file", path="/secrets.json")
secrets(backend="file", path="/secrets.json")

target.local(name="local")
deploy(name="test", targets=["local"], steps=[])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)
	src.Files["/secrets.json"] = []byte(`{}`)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for double secrets() call, got nil")
	}
}

// TestSecrets_UnknownBackend verifies secrets() rejects unknown backends.
func TestSecrets_UnknownBackend(t *testing.T) {
	cfgStr := `
secrets(backend="vault")

target.local(name="local")
deploy(name="test", targets=["local"], steps=[])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}
}

// TestSecrets_MissingFile verifies secrets() errors when the file doesn't exist.
func TestSecrets_MissingFile(t *testing.T) {
	cfgStr := `
secrets(backend="file", path="nonexistent.json")

target.local(name="local")
deploy(name="test", targets=["local"], steps=[])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for missing secrets file, got nil")
	}
}

// Age backend integration tests
// -----------------------------------------------------------------------------

func ageTestKeypair(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generating keypair: %v", err)
	}
	return id
}

func ageEncryptedJSON(t *testing.T, id *age.X25519Identity, kv map[string]string) []byte {
	t.Helper()
	encrypted := make(map[string]string, len(kv))
	for k, v := range kv {
		enc, err := secret.EncryptValue(v, []age.Recipient{id.Recipient()})
		if err != nil {
			t.Fatalf("encrypting %q: %v", k, err)
		}
		encrypted[k] = enc
	}
	data, err := json.Marshal(encrypted)
	if err != nil {
		t.Fatalf("marshaling JSON: %v", err)
	}
	return data
}

// TestSecret_AgeBackend verifies age-encrypted secrets flow through to templates.
func TestSecret_AgeBackend(t *testing.T) {
	id := ageTestKeypair(t)

	cfgStr := `
secrets(backend="age", path="/secrets.age.json")

target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="age-secret",
            src=inline("pass={{.db_pass}}"),
            dest="/out.txt",
            data={
                "values": {
                    "db_pass": secret("db_pass"),
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/secrets.age.json"] = ageEncryptedJSON(t, id, map[string]string{
		"db_pass": "hunter2",
	})
	src.Env["SCAMPI_AGE_KEY"] = id.String()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	err = e.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	data, ok := tgt.Files["/out.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	if string(data) != "pass=hunter2" {
		t.Errorf("unexpected content: got %q, want %q", data, "pass=hunter2")
	}
}

// TestSecret_AgeNotFound verifies a missing key in age backend produces an abort.
func TestSecret_AgeNotFound(t *testing.T) {
	id := ageTestKeypair(t)

	cfgStr := `
secrets(backend="age", path="/secrets.age.json")

target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="missing-age-secret",
            src=inline("{{.token}}"),
            dest="/out.txt",
            data={
                "values": {
                    "token": secret("missing_key"),
                },
            },
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)
	src.Files["/secrets.age.json"] = ageEncryptedJSON(t, id, map[string]string{})
	src.Env["SCAMPI_AGE_KEY"] = id.String()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}

	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}

	var notFound *star.SecretNotFoundError
	found := false
	for _, cause := range abort.Causes {
		if errors.As(cause, &notFound) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SecretNotFoundError in causes, got: %v", abort.Causes)
	}
}

// TestSecret_AgeMissingIdentity verifies a clear error when no age identity is available.
func TestSecret_AgeMissingIdentity(t *testing.T) {
	id := ageTestKeypair(t)

	cfgStr := `
secrets(backend="age", path="/secrets.age.json")

target.local(name="local")
deploy(name="test", targets=["local"], steps=[])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)
	src.Files["/secrets.age.json"] = ageEncryptedJSON(t, id, map[string]string{})
	// Point SCAMPI_AGE_KEY_FILE at a nonexistent path to block the default-file
	// fallback (which would succeed if ~/.config/scampi/age.key happens to exist).
	src.Env["SCAMPI_AGE_KEY_FILE"] = "/nonexistent/scampi-test/age.key"

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for missing identity, got nil")
	}

	var abort engine.AbortError
	if !errors.As(err, &abort) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}

	var cfgErr *star.SecretsConfigError
	found := false
	for _, cause := range abort.Causes {
		if errors.As(cause, &cfgErr) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SecretsConfigError in causes, got: %v", abort.Causes)
	}
}

// TestSecret_AgeMissingFile verifies an error when the age secrets file doesn't exist.
func TestSecret_AgeMissingFile(t *testing.T) {
	cfgStr := `
secrets(backend="age", path="nonexistent.age.json")

target.local(name="local")
deploy(name="test", targets=["local"], steps=[])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)
	// No secrets file, but set identity so we get past identity resolution
	id := ageTestKeypair(t)
	src.Env["SCAMPI_AGE_KEY"] = id.String()

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for missing secrets file, got nil")
	}
}
