package test

import (
	"context"
	"errors"
	"testing"

	"godoit.dev/doit/diagnostic"
	"godoit.dev/doit/engine"
	"godoit.dev/doit/source"
	"godoit.dev/doit/spec"
	"godoit.dev/doit/target/ssh"
)

// TestEnvOverride_EnvOnlyField tests that a field with no default
// can be filled from an environment variable.
func TestEnvOverride_EnvOnlyField(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

target: builtin.ssh & {
	host:     string @env(SSH_HOST)
	port:     22
	user:     "testuser"
	insecure: true
}

steps: []
`
	src := source.NewMemSource()
	src.Files["/config.cue"] = []byte(cfgStr)
	src.Env["SSH_HOST"] = "test.example.com"

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify the env value was applied to the SSH config
	sshCfg, ok := cfg.Target.Config.(*ssh.Config)
	if !ok {
		t.Fatalf("expected *ssh.Config, got %T", cfg.Target.Config)
	}
	if sshCfg.Host != "test.example.com" {
		t.Errorf("expected host=test.example.com, got %q", sshCfg.Host)
	}
}

// TestEnvOverride_ConcreteValueOverride tests that a concrete value
// in the config can be overridden by an environment variable.
func TestEnvOverride_ConcreteValueOverride(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

target: builtin.ssh & {
	host:     "localhost"
	port:     2222 @env(SSH_PORT)
	user:     "testuser"
	insecure: true
}

steps: []
`
	src := source.NewMemSource()
	src.Files["/config.cue"] = []byte(cfgStr)
	src.Env["SSH_PORT"] = "3333"

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	sshCfg, ok := cfg.Target.Config.(*ssh.Config)
	if !ok {
		t.Fatalf("expected *ssh.Config, got %T", cfg.Target.Config)
	}
	if sshCfg.Port != 3333 {
		t.Errorf("expected port=3333, got %d", sshCfg.Port)
	}
}

// TestEnvOverride_ConcreteValueNoEnv tests that without env var set,
// the concrete config value is used.
func TestEnvOverride_ConcreteValueNoEnv(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

target: builtin.ssh & {
	host:     "localhost"
	port:     2222 @env(SSH_PORT)
	user:     "testuser"
	insecure: true
}

steps: []
`
	src := source.NewMemSource()
	src.Files["/config.cue"] = []byte(cfgStr)
	// SSH_PORT not set

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	sshCfg, ok := cfg.Target.Config.(*ssh.Config)
	if !ok {
		t.Fatalf("expected *ssh.Config, got %T", cfg.Target.Config)
	}
	if sshCfg.Port != 2222 {
		t.Errorf("expected port=2222 (config value), got %d", sshCfg.Port)
	}
}

// TestEnvOverride_DefaultWithEnv tests that a default value with env attribute
// uses the default when env is not set, and env value when set.
func TestEnvOverride_DefaultWithEnv(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

target: builtin.ssh & {
	host:     *"default.host.com" | string @env(SSH_HOST)
	port:     22
	user:     "testuser"
	insecure: true
}

steps: []
`
	t.Run("env not set uses default", func(t *testing.T) {
		src := source.NewMemSource()
		src.Files["/config.cue"] = []byte(cfgStr)
		// No env var set

		rec := &recordingDisplayer{}
		em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
		store := spec.NewSourceStore()

		ctx := context.Background()
		cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		sshCfg, ok := cfg.Target.Config.(*ssh.Config)
		if !ok {
			t.Fatalf("expected *ssh.Config, got %T", cfg.Target.Config)
		}
		if sshCfg.Host != "default.host.com" {
			t.Errorf("expected host=default.host.com, got %q", sshCfg.Host)
		}
	})

	t.Run("env set overrides default", func(t *testing.T) {
		src := source.NewMemSource()
		src.Files["/config.cue"] = []byte(cfgStr)
		src.Env["SSH_HOST"] = "env.host.com"

		rec := &recordingDisplayer{}
		em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
		store := spec.NewSourceStore()

		ctx := context.Background()
		cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		sshCfg, ok := cfg.Target.Config.(*ssh.Config)
		if !ok {
			t.Fatalf("expected *ssh.Config, got %T", cfg.Target.Config)
		}
		if sshCfg.Host != "env.host.com" {
			t.Errorf("expected host=env.host.com, got %q", sshCfg.Host)
		}
	})
}

// TestEnvOverride_MissingRequiredEnv tests that a missing required env-only field
// produces an appropriate error with env hint.
func TestEnvOverride_MissingRequiredEnv(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

target: builtin.ssh & {
	host:     string @env(SSH_HOST)
	port:     22
	user:     "testuser"
	insecure: true
}

steps: []
`
	src := source.NewMemSource()
	src.Files["/config.cue"] = []byte(cfgStr)
	// SSH_HOST not set - should fail

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)

	// Should fail with AbortError
	var abortErr engine.AbortError
	if !errors.As(err, &abortErr) {
		t.Fatalf("expected AbortError, got %T: %v", err, err)
	}

	// Check that diagnostic mentions the env var
	found := false
	for _, d := range rec.engineDiagnostics {
		if d.Detail.Template.ID == "config.MissingField" {
			found = true
			// Verify the hint mentions SSH_HOST
			data, ok := d.Detail.Template.Data.(interface{ Env() string })
			if ok && data.Env() != "SSH_HOST" {
				t.Errorf("expected env hint for SSH_HOST")
			}
		}
	}
	if !found {
		t.Logf("Diagnostics: %+v", rec.engineDiagnostics)
		t.Error("expected config.MissingField diagnostic")
	}
}

// TestEnvOverride_InvalidIntValue tests that invalid int env values produce errors.
func TestEnvOverride_InvalidIntValue(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

target: builtin.ssh & {
	host:     "localhost"
	port:     int @env(SSH_PORT)
	user:     "testuser"
	insecure: true
}

steps: []
`
	src := source.NewMemSource()
	src.Files["/config.cue"] = []byte(cfgStr)
	src.Env["SSH_PORT"] = "not-a-number"

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	_, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)

	// Should fail
	if err == nil {
		t.Fatal("expected error for invalid int value")
	}
}

// TestEnvOverride_BoolField tests that boolean fields can be set via env.
func TestEnvOverride_BoolField(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

target: builtin.ssh & {
	host:     "localhost"
	port:     22
	user:     "testuser"
	insecure: bool @env(SSH_INSECURE)
}

steps: []
`
	src := source.NewMemSource()
	src.Files["/config.cue"] = []byte(cfgStr)
	src.Env["SSH_INSECURE"] = "true"

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	sshCfg, ok := cfg.Target.Config.(*ssh.Config)
	if !ok {
		t.Fatalf("expected *ssh.Config, got %T", cfg.Target.Config)
	}
	if !sshCfg.Insecure {
		t.Error("expected insecure=true")
	}
}

// TestEnvOverride_BoolFieldFalse tests that boolean false can be set via env.
func TestEnvOverride_BoolFieldFalse(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

target: builtin.ssh & {
	host:     "localhost"
	port:     22
	user:     "testuser"
	insecure: true @env(SSH_INSECURE)
}

steps: []
`
	src := source.NewMemSource()
	src.Files["/config.cue"] = []byte(cfgStr)
	src.Env["SSH_INSECURE"] = "false"

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	sshCfg, ok := cfg.Target.Config.(*ssh.Config)
	if !ok {
		t.Fatalf("expected *ssh.Config, got %T", cfg.Target.Config)
	}
	if sshCfg.Insecure {
		t.Error("expected insecure=false (env override)")
	}
}

// TestEnvOverride_MultipleFields tests multiple env overrides on same target.
func TestEnvOverride_MultipleFields(t *testing.T) {
	cfgStr := `
package test

import "godoit.dev/doit/builtin"

target: builtin.ssh & {
	host:     string @env(SSH_HOST)
	port:     int    @env(SSH_PORT)
	user:     string @env(SSH_USER)
	insecure: true
}

steps: []
`
	src := source.NewMemSource()
	src.Files["/config.cue"] = []byte(cfgStr)
	src.Env["SSH_HOST"] = "multi.test.com"
	src.Env["SSH_PORT"] = "2222"
	src.Env["SSH_USER"] = "envuser"

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := spec.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.cue", store, src)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	sshCfg, ok := cfg.Target.Config.(*ssh.Config)
	if !ok {
		t.Fatalf("expected *ssh.Config, got %T", cfg.Target.Config)
	}

	if sshCfg.Host != "multi.test.com" {
		t.Errorf("expected host=multi.test.com, got %q", sshCfg.Host)
	}
	if sshCfg.Port != 2222 {
		t.Errorf("expected port=2222, got %d", sshCfg.Port)
	}
	if sshCfg.User != "envuser" {
		t.Errorf("expected user=envuser, got %q", sshCfg.User)
	}
}
