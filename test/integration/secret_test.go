// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"filippo.io/age"

	"scampi.dev/scampi/internal/controller"
	"scampi.dev/scampi/internal/diagnostic"
	"scampi.dev/scampi/internal/diagnostic/event"
	"scampi.dev/scampi/internal/engine"
	"scampi.dev/scampi/internal/secret"
	"scampi.dev/scampi/internal/target"
	"scampi.dev/scampi/test/harness"
)

// Test_Secret_ResolvesIntoTemplateData verifies that resolver.get() values
// flow through to template rendering.
func Test_Secret_ResolvesIntoTemplateData(t *testing.T) {
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
	ctl := controller.NewMem()
	tgt := target.NewMemTarget()

	ctl.Files["/secrets.json"] = []byte(`{"db_pass": "hunter2"}`)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewInputStore()

	e, err := loadAndResolve(t, cfgStr, ctl, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v\nrecorder: %s", err, rec)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(t.Context(), em))
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

// Test_Secret_ErrorsOnMissingKey verifies that a missing secret produces an abort.
func Test_Secret_ErrorsOnMissingKey(t *testing.T) {
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
	ctl := controller.NewMem()
	ctl.Files["/config.scampi"] = []byte(cfgStr)
	ctl.Files["/secrets.json"] = []byte(`{}`)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewInputStore()

	ctx := t.Context()
	_, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}

	if !recContains(rec, "not found") && !recContains(rec, "secret") {
		t.Fatalf("expected diagnostic about missing secret, got events: %v", rec.Diagnostics)
	}
}

// Test_Secrets_FromFileResolvesValues verifies secrets.from_file configures the backend.
func Test_Secrets_FromFileResolvesValues(t *testing.T) {
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
	ctl := controller.NewMem()
	tgt := target.NewMemTarget()

	ctl.Files["/my-secrets.json"] = []byte(`{"api_token": "tok-abc123"}`)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewInputStore()

	e, err := loadAndResolve(t, cfgStr, ctl, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(t.Context(), em))
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

// Test_Secrets_ErrorsOnMissingFile verifies from_file errors when the file doesn't exist.
func Test_Secrets_ErrorsOnMissingFile(t *testing.T) {
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
	ctl := controller.NewMem()
	ctl.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewInputStore()

	ctx := t.Context()
	_, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
	if err == nil {
		t.Fatal("expected error for missing secrets file, got nil")
	}
}

// Test_Secrets_ResolvesFromMultipleResolvers verifies multiple resolvers can coexist.
func Test_Secrets_ResolvesFromMultipleResolvers(t *testing.T) {
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
	ctl := controller.NewMem()
	tgt := target.NewMemTarget()

	ctl.Files["/secrets1.json"] = []byte(`{"key_a": "val_a"}`)
	ctl.Files["/secrets2.json"] = []byte(`{"key_b": "val_b"}`)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewInputStore()

	e, err := loadAndResolve(t, cfgStr, ctl, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(t.Context(), em))
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

// Test_Secret_AgeDecryptsIntoTemplateData verifies age-encrypted secrets flow through to templates.
func Test_Secret_AgeDecryptsIntoTemplateData(t *testing.T) {
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
	ctl := controller.NewMem()
	tgt := target.NewMemTarget()

	ctl.Files["/secrets.age.json"] = ageEncryptedJSON(t, id, map[string]string{
		"db_pass": "hunter2",
	})
	ctl.Env["SCAMPI_AGE_KEY"] = id.String()

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewInputStore()

	e, err := loadAndResolve(t, cfgStr, ctl, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(t.Context(), em))
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

// Test_Secret_AgeErrorsOnMissingKey verifies a missing key in age backend produces an abort.
func Test_Secret_AgeErrorsOnMissingKey(t *testing.T) {
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
	ctl := controller.NewMem()
	ctl.Files["/config.scampi"] = []byte(cfgStr)
	ctl.Files["/secrets.age.json"] = ageEncryptedJSON(t, id, map[string]string{})
	ctl.Env["SCAMPI_AGE_KEY"] = id.String()

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewInputStore()

	ctx := t.Context()
	_, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
	if err == nil {
		t.Fatal("expected error for missing secret, got nil")
	}

	if !recContains(rec, "not found") && !recContains(rec, "secret") {
		t.Fatalf("expected diagnostic about missing secret, got events: %v", rec.Diagnostics)
	}
}

// Test_Secret_AgeFallsBackToPlaceholderWithoutIdentity verifies that missing identity falls back to
// a placeholder backend (knows keys, can't decrypt).
func Test_Secret_AgeFallsBackToPlaceholderWithoutIdentity(t *testing.T) {
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
	ctl := controller.NewMem()
	tgt := target.NewMemTarget()

	ctl.Files["/secrets.age.json"] = ageEncryptedJSON(t, id, map[string]string{
		"db_pass": "hunter2",
	})
	// No SCAMPI_AGE_KEY - block the default-file fallback
	ctl.Env["SCAMPI_AGE_KEY_FILE"] = "/nonexistent/scampi-test/age.key"

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewInputStore()

	// Placeholder returns "<secret>" as the value. The template
	// should render with that placeholder value.
	e, err := loadAndResolve(t, cfgStr, ctl, tgt, em, store)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer e.Close()

	_, err = e.Apply(diagnostic.NewCtx(t.Context(), em))
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

// Test_Secret_AgeErrorsOnMissingFile verifies an error when the age secrets file doesn't exist.
func Test_Secret_AgeErrorsOnMissingFile(t *testing.T) {
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
	ctl := controller.NewMem()
	ctl.Files["/config.scampi"] = []byte(cfgStr)
	id := ageTestKeypair(t)
	ctl.Env["SCAMPI_AGE_KEY"] = id.String()

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewInputStore()

	ctx := t.Context()
	_, err := engine.LoadConfig(diagnostic.NewCtx(ctx, em), "/config.scampi", store, ctl)
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
