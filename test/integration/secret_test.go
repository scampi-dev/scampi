// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"filippo.io/age"

	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/secret"
	"scampi.dev/scampi/internal/source"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/test/harness"
)

// TestSecret_ResolvesIntoTemplateData verifies that resolver.get() values
// flow through to template rendering.
func TestSecret_ResolvesIntoTemplateData(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/secrets"

let host = local.target { name = "local" }
let resolver = secrets.from_file(path = "/secrets.json")

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "secret-template"
    src = posix.source_inline { content = "pass={{.db_pass}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "db_pass": resolver.get("db_pass"),
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/secrets.json"] = []byte(`{"db_pass": "hunter2"}`)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v\nrecorder: %s", err, rec)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(context.Background(), em))
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
module main
import "std"
import "std/posix"
import "std/local"
import "std/secrets"

let host = local.target { name = "local" }
let resolver = secrets.from_file(path = "/secrets.json")

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "missing-secret"
    src = posix.source_inline { content = "{{.token}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "token": resolver.get("missing_key"),
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)
	src.Files["/secrets.json"] = []byte(`{}`)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}

	if !recContains(rec, "not found") && !recContains(rec, "secret") {
		t.Fatalf("expected diagnostic about missing secret, got events: %v", rec.Diagnostics)
	}
}

// TestSecrets_FileBackend verifies secrets.from_file configures the backend.
func TestSecrets_FileBackend(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/secrets"

let host = local.target { name = "local" }
let resolver = secrets.from_file(path = "/my-secrets.json")

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "explicit-backend"
    src = posix.source_inline { content = "token={{.api_token}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "api_token": resolver.get("api_token"),
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/my-secrets.json"] = []byte(`{"api_token": "tok-abc123"}`)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(context.Background(), em))
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

// TestSecrets_MissingFile verifies from_file errors when the file doesn't exist.
func TestSecrets_MissingFile(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/secrets"

let host = local.target { name = "local" }
let resolver = secrets.from_file(path = "nonexistent.json")

std.deploy(name = "test", targets = [host]) {}
`
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err == nil {
		t.Fatal("expected error for missing secrets file, got nil")
	}
}

// TestSecrets_MultipleResolvers verifies multiple resolvers can coexist.
func TestSecrets_MultipleResolvers(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/secrets"

let host = local.target { name = "local" }
let first = secrets.from_file(path = "/secrets1.json")
let second = secrets.from_file(path = "/secrets2.json")

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "multi-resolver"
    src = posix.source_inline { content = "a={{.a}} b={{.b}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "a": first.get("key_a"),
        "b": second.get("key_b"),
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/secrets1.json"] = []byte(`{"key_a": "val_a"}`)
	src.Files["/secrets2.json"] = []byte(`{"key_b": "val_b"}`)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(context.Background(), em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	data, ok := tgt.Files["/out.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	if string(data) != "a=val_a b=val_b" {
		t.Errorf("unexpected content: got %q, want %q", data, "a=val_a b=val_b")
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
module main
import "std"
import "std/posix"
import "std/local"
import "std/secrets"

let host = local.target { name = "local" }
let resolver = secrets.from_age(path = "/secrets.age.json")

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "age-secret"
    src = posix.source_inline { content = "pass={{.db_pass}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "db_pass": resolver.get("db_pass"),
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/secrets.age.json"] = ageEncryptedJSON(t, id, map[string]string{
		"db_pass": "hunter2",
	})
	src.Env["SCAMPI_AGE_KEY"] = id.String()

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(context.Background(), em))
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
module main
import "std"
import "std/posix"
import "std/local"
import "std/secrets"

let host = local.target { name = "local" }
let resolver = secrets.from_age(path = "/secrets.age.json")

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "missing-age-secret"
    src = posix.source_inline { content = "{{.token}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "token": resolver.get("missing_key"),
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)
	src.Files["/secrets.age.json"] = ageEncryptedJSON(t, id, map[string]string{})
	src.Env["SCAMPI_AGE_KEY"] = id.String()

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}

	if !recContains(rec, "not found") && !recContains(rec, "secret") {
		t.Fatalf("expected diagnostic about missing secret, got events: %v", rec.Diagnostics)
	}
}

// TestSecret_AgePlaceholder verifies that missing identity falls back to
// a placeholder backend (knows keys, can't decrypt).
func TestSecret_AgePlaceholder(t *testing.T) {
	id := ageTestKeypair(t)

	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/secrets"

let host = local.target { name = "local" }
let resolver = secrets.from_age(path = "/secrets.age.json")

std.deploy(name = "test", targets = [host]) {
  posix.template {
    desc = "placeholder-secret"
    src = posix.source_inline { content = "pass={{.db_pass}}" }
    dest = "/out.txt"
    data = {
      "values": {
        "db_pass": resolver.get("db_pass"),
      },
    }
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	src := source.NewMemSource()
	tgt := target.NewMemTarget()

	src.Files["/secrets.age.json"] = ageEncryptedJSON(t, id, map[string]string{
		"db_pass": "hunter2",
	})
	// No SCAMPI_AGE_KEY — block the default-file fallback
	src.Env["SCAMPI_AGE_KEY_FILE"] = "/nonexistent/scampi-test/age.key"

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	// Placeholder returns "<secret>" as the value. The template
	// should render with that placeholder value.
	e, err := loadAndResolve(t, cfgStr, src, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(context.Background(), em))
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	data, ok := tgt.Files["/out.txt"]
	if !ok {
		t.Fatal("destination file not created")
	}
	if string(data) != "pass=<secret>" {
		t.Errorf("unexpected content: got %q, want %q", data, "pass=<secret>")
	}
}

// TestSecret_AgeMissingFile verifies an error when the age secrets file doesn't exist.
func TestSecret_AgeMissingFile(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/secrets"

let host = local.target { name = "local" }
let resolver = secrets.from_age(path = "nonexistent.age.json")

std.deploy(name = "test", targets = [host]) {}
`
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)
	id := ageTestKeypair(t)
	src.Env["SCAMPI_AGE_KEY"] = id.String()

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, src)
	if err == nil {
		t.Fatal("expected error for missing secrets file, got nil")
	}
}

func recContains(rec *harness.RecordingDisplayer, substr string) bool {
	for _, ev := range rec.Diagnostics {
		tmpl := event.TemplateOf(ev)
		if strings.Contains(tmpl.Text, substr) {
			return true
		}
		if strings.Contains(fmt.Sprintf("%+v", tmpl.Data), substr) {
			return true
		}
	}
	return false
}
