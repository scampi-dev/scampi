// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"scampi.dev/scampi/capability"
	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/rest"
	"scampi.dev/scampi/test/harness"
)

// Config loading
// -----------------------------------------------------------------------------

func TestRESTTarget_ConfigLoads(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/rest"

let api = rest.target { name = "api", base_url = "http://localhost:8080/api" }

std.deploy(name = "test", targets = [api]) {
  posix.run { apply = "echo ok", check = "true" }
}
`
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	cfg, err := engine.LoadConfig(context.Background(), em, "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	ti, ok := cfg.Targets["api"]
	if !ok {
		t.Fatal("target 'api' not found in config")
	}
	if ti.Type.Kind() != "rest" {
		t.Fatalf("expected target kind 'rest', got %q", ti.Type.Kind())
	}
	restCfg, ok := ti.Config.(*rest.Config)
	if !ok {
		t.Fatalf("expected *rest.Config, got %T", ti.Config)
	}
	if restCfg.BaseURL != "http://localhost:8080/api" {
		t.Fatalf("expected base_url 'http://localhost:8080/api', got %q", restCfg.BaseURL)
	}
}

func TestRESTTarget_ConfigWithBasicAuth(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/rest"

let api = rest.target {
  name = "api"
  base_url = "http://localhost/api"
  auth = rest.basic { user = "admin", password = "secret" }
}

std.deploy(name = "test", targets = [api]) {
  posix.run { apply = "echo ok", check = "true" }
}
`
	cfg := loadConfig(t, cfgStr)
	restCfg := cfg.Targets["api"].Config.(*rest.Config)

	auth, ok := restCfg.Auth.(rest.BasicAuthConfig)
	if !ok {
		t.Fatalf("expected BasicAuthConfig, got %T", restCfg.Auth)
	}
	if auth.User != "admin" || auth.Password != "secret" {
		t.Fatalf("unexpected basic auth: user=%q password=%q", auth.User, auth.Password)
	}
}

func TestRESTTarget_ConfigWithBearerAuth(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/rest"

let api = rest.target {
  name = "api"
  base_url = "http://localhost/api"
  auth = rest.bearer {
    token_endpoint = "/tokens"
    identity = "admin@test.com"
    secret = "pass123"
  }
}

std.deploy(name = "test", targets = [api]) {
  posix.run { apply = "echo ok", check = "true" }
}
`
	cfg := loadConfig(t, cfgStr)
	restCfg := cfg.Targets["api"].Config.(*rest.Config)

	auth, ok := restCfg.Auth.(rest.BearerAuthConfig)
	if !ok {
		t.Fatalf("expected BearerAuthConfig, got %T", restCfg.Auth)
	}
	if auth.TokenEndpoint != "/tokens" {
		t.Fatalf("expected token_endpoint '/tokens', got %q", auth.TokenEndpoint)
	}
	if auth.Identity != "admin@test.com" {
		t.Fatalf("expected identity 'admin@test.com', got %q", auth.Identity)
	}
}

func TestRESTTarget_ConfigWithHeaderAuth(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/rest"

let api = rest.target {
  name = "api"
  base_url = "http://localhost/api"
  auth = rest.header { name = "X-API-Key", value = "my-key" }
}

std.deploy(name = "test", targets = [api]) {
  posix.run { apply = "echo ok", check = "true" }
}
`
	cfg := loadConfig(t, cfgStr)
	restCfg := cfg.Targets["api"].Config.(*rest.Config)

	auth, ok := restCfg.Auth.(rest.HeaderAuthConfig)
	if !ok {
		t.Fatalf("expected HeaderAuthConfig, got %T", restCfg.Auth)
	}
	if auth.Name != "X-API-Key" || auth.Value != "my-key" {
		t.Fatalf("unexpected header auth: name=%q value=%q", auth.Name, auth.Value)
	}
}

func TestRESTTarget_ConfigWithTLSInsecure(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/rest"

let api = rest.target { name = "api", base_url = "https://localhost/api", tls = rest.tls_insecure {} }

std.deploy(name = "test", targets = [api]) {
  posix.run { apply = "echo ok", check = "true" }
}
`
	cfg := loadConfig(t, cfgStr)
	restCfg := cfg.Targets["api"].Config.(*rest.Config)

	if _, ok := restCfg.TLS.(rest.InsecureTLSConfig); !ok {
		t.Fatalf("expected InsecureTLSConfig, got %T", restCfg.TLS)
	}
}

func TestRESTTarget_ConfigWithTLSSecure(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/rest"

let api = rest.target { name = "api", base_url = "https://localhost/api", tls = rest.tls_secure {} }

std.deploy(name = "test", targets = [api]) {
  posix.run { apply = "echo ok", check = "true" }
}
`
	cfg := loadConfig(t, cfgStr)
	restCfg := cfg.Targets["api"].Config.(*rest.Config)

	if _, ok := restCfg.TLS.(rest.SecureTLSConfig); !ok {
		t.Fatalf("expected SecureTLSConfig, got %T", restCfg.TLS)
	}
}

func TestRESTTarget_ConfigInvalidAuth(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/rest"

let api = rest.target { name = "api", base_url = "http://localhost/api", auth = "not-an-auth" }

std.deploy(name = "test", targets = [api]) {
  posix.run { apply = "echo ok", check = "true" }
}
`
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	_, err := engine.LoadConfig(context.Background(), em, "/config.scampi", store, src)
	if err == nil {
		t.Fatal("expected error for invalid auth type")
	}
}

func TestRESTTarget_ConfigEmptyName(t *testing.T) {
	t.Skip("empty name validation needs to move to engine Create()")
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"
import "std/rest"

let api = rest.target { name = "", base_url = "http://localhost/api" }

std.deploy(name = "test", targets = [api]) {
  posix.run { apply = "echo ok", check = "true" }
}
`
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	_, err := engine.LoadConfig(context.Background(), em, "/config.scampi", store, src)
	if err == nil {
		t.Fatal("expected error for empty target name")
	}
}

// Capability mismatch
// -----------------------------------------------------------------------------

func TestRESTTarget_RejectsPOSIXSteps(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/posix"
import "std/local"

let host = local.target { name = "local" }

std.deploy(name = "test", targets = [host]) {
  posix.copy {
    src = posix.source_local { path = "/a" }
    dest = "/b"
    perm = "0644"
    owner = "user"
    group = "group"
  }
}
`
	tgt := newRESTOnlyTarget()
	assertCapabilityMismatch(t, cfgStr, tgt)
}

// Target creation and HTTPClient
// -----------------------------------------------------------------------------

func TestRESTTarget_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	tgt := createRESTTarget(t, &rest.Config{BaseURL: srv.URL})

	if !tgt.Capabilities().HasAll(capability.REST) {
		t.Fatal("expected REST capability")
	}
	if tgt.Capabilities().HasAny(capability.POSIX) {
		t.Fatal("REST target should not have POSIX capabilities")
	}
}

func TestRESTTarget_Do_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/items" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer srv.Close()

	tgt := createRESTTarget(t, &rest.Config{BaseURL: srv.URL})
	client := target.Must[target.HTTPClient]("test", tgt)

	resp, err := client.Do(context.Background(), target.HTTPRequest{
		Method: "GET",
		Path:   "/items",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != `[{"id":1}]` {
		t.Fatalf("unexpected body: %s", resp.Body)
	}
}

func TestRESTTarget_Do_POST(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected json content-type, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	tgt := createRESTTarget(t, &rest.Config{BaseURL: srv.URL})
	client := target.Must[target.HTTPClient]("test", tgt)

	resp, err := client.Do(context.Background(), target.HTTPRequest{
		Method:  "POST",
		Path:    "/items",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    []byte(`{"name":"test"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
}

func TestRESTTarget_Do_TrailingSlashNormalized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/items" {
			t.Fatalf("expected /v2/items, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tgt := createRESTTarget(t, &rest.Config{BaseURL: srv.URL + "/"})
	client := target.Must[target.HTTPClient]("test", tgt)

	resp, err := client.Do(context.Background(), target.HTTPRequest{Method: "GET", Path: "/v2/items"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// Auth transports (end-to-end through target)
// -----------------------------------------------------------------------------

func TestRESTTarget_BasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tgt := createRESTTarget(t, &rest.Config{
		BaseURL: srv.URL,
		Auth:    rest.BasicAuthConfig{User: "admin", Password: "secret"},
	})
	client := target.Must[target.HTTPClient]("test", tgt)

	resp, err := client.Do(context.Background(), target.HTTPRequest{Method: "GET", Path: "/test"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRESTTarget_HeaderAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "my-key-123" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tgt := createRESTTarget(t, &rest.Config{
		BaseURL: srv.URL,
		Auth:    rest.HeaderAuthConfig{Name: "X-API-Key", Value: "my-key-123"},
	})
	client := target.Must[target.HTTPClient]("test", tgt)

	resp, err := client.Do(context.Background(), target.HTTPRequest{Method: "GET", Path: "/test"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRESTTarget_BearerAuth(t *testing.T) {
	var tokenFetches atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tokens" {
			tokenFetches.Add(1)
			var creds map[string]string
			if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if creds["identity"] != "admin@test.com" || creds["secret"] != "pass123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "valid-token"})
			return
		}

		if r.Header.Get("Authorization") != "Bearer valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tgt := createRESTTarget(t, &rest.Config{
		BaseURL: srv.URL,
		Auth: rest.BearerAuthConfig{
			TokenEndpoint: "/tokens",
			Identity:      "admin@test.com",
			Secret:        "pass123",
		},
	})
	client := target.Must[target.HTTPClient]("test", tgt)

	// First request triggers token fetch.
	resp, err := client.Do(context.Background(), target.HTTPRequest{Method: "GET", Path: "/data"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Second request reuses cached token.
	resp2, err := client.Do(context.Background(), target.HTTPRequest{Method: "GET", Path: "/other"})
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	if got := tokenFetches.Load(); got != 1 {
		t.Fatalf("expected 1 token fetch, got %d", got)
	}
}

func TestRESTTarget_BearerAuth_AccessTokenField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "oauth-tok"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer oauth-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tgt := createRESTTarget(t, &rest.Config{
		BaseURL: srv.URL,
		Auth: rest.BearerAuthConfig{
			TokenEndpoint: "/oauth/token",
			Identity:      "client",
			Secret:        "secret",
		},
	})
	client := target.Must[target.HTTPClient]("test", tgt)

	resp, err := client.Do(context.Background(), target.HTTPRequest{Method: "GET", Path: "/api"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRESTTarget_BearerAuth_ReauthOn401(t *testing.T) {
	var apiCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tokens" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "fresh-token"})
			return
		}

		n := apiCalls.Add(1)
		auth := r.Header.Get("Authorization")

		// First API call: return 401 to trigger re-auth.
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if auth != "Bearer fresh-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tgt := createRESTTarget(t, &rest.Config{
		BaseURL: srv.URL,
		Auth: rest.BearerAuthConfig{
			TokenEndpoint: "/tokens",
			Identity:      "user",
			Secret:        "pass",
		},
	})
	client := target.Must[target.HTTPClient]("test", tgt)

	resp, err := client.Do(context.Background(), target.HTTPRequest{Method: "GET", Path: "/data"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after re-auth, got %d", resp.StatusCode)
	}
}

// rest.request step
// -----------------------------------------------------------------------------

func TestRestRequest_POST_WithStatusCheck(t *testing.T) {
	var created bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/items":
			if created {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == "POST" && r.URL.Path == "/items":
			created = true
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.request {
    desc = "create item"
    method = "POST"
    path = "/items"
    body = rest.body_json { data = {"name": "test"} }
    check = rest.status { code = 200 }
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if !created {
		t.Fatal("POST was not executed")
	}
	if rec.CountChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.CountChangedOps())
	}
}

func TestRestRequest_POST_AlreadySatisfied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/items" {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.request {
    desc = "create item"
    method = "POST"
    path = "/items"
    check = rest.status { code = 200 }
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if rec.CountChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops (already satisfied), got %d", rec.CountChangedOps())
	}
}

func TestRestRequest_WithJQCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/hosts" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"domain": "other.com"}]`))
			return
		}
		if r.Method == "POST" && r.URL.Path == "/hosts" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.request {
    desc = "create host"
    method = "POST"
    path = "/hosts"
    body = rest.body_json { data = {"domain": "example.com"} }
    check = rest.jq { expr = ".[] | select(.domain == \"example.com\")" }
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if rec.CountChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.CountChangedOps())
	}
}

func TestRestRequest_JQCheck_AlreadySatisfied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/hosts" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"domain": "example.com"}]`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.request {
    desc = "create host"
    method = "POST"
    path = "/hosts"
    check = rest.jq { expr = ".[] | select(.domain == \"example.com\")" }
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if rec.CountChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops (already satisfied), got %d", rec.CountChangedOps())
	}
}

func TestRestRequest_NoCheck_AlwaysExecutes(t *testing.T) {
	var called bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" && r.URL.Path == "/config" {
			called = true
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.request {
    desc = "update config"
    method = "PUT"
    path = "/config"
    body = rest.body_json { data = {"key": "value"} }
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if !called {
		t.Fatal("PUT was not executed")
	}
	if rec.CountChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.CountChangedOps())
	}
}

func TestRestRequest_ExecuteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.request {
    method = "POST"
    path = "/items"
    check = rest.status { code = 200 }
  }
}
`, srv.URL)

	_, err := applyREST(t, cfgStr, srv.URL)
	if err == nil {
		t.Fatal("expected error from 500 response")
	}
}

func applyREST(t *testing.T, cfgStr, baseURL string) (*harness.RecordingDisplayer, error) {
	t.Helper()

	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)

	tgt := createRESTTarget(t, &rest.Config{BaseURL: baseURL})

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
	if err != nil {
		return rec, err
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		return rec, err
	}

	resolved.Target = harness.MockTargetInstance(tgt)

	e, err := engine.NewWithTarget(ctx, src, resolved, em, tgt)
	if err != nil {
		return rec, err
	}
	defer e.Close()

	return rec, e.Apply(ctx)
}

// Helpers
// -----------------------------------------------------------------------------

func loadConfig(t *testing.T, cfgStr string) spec.Config {
	t.Helper()
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	cfg, err := engine.LoadConfig(context.Background(), em, "/config.scampi", store, src)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	return cfg
}

func createRESTTarget(t *testing.T, cfg *rest.Config) target.Target {
	t.Helper()
	if cfg.Auth == nil {
		cfg.Auth = rest.NoAuthConfig{}
	}
	if cfg.TLS == nil {
		cfg.TLS = rest.SecureTLSConfig{}
	}
	inst := spec.TargetInstance{Config: cfg}
	tgt, err := rest.REST{}.Create(context.Background(), nil, inst)
	if err != nil {
		t.Fatal(err)
	}
	return tgt
}

// restOnlyTarget wraps MemTarget but only exposes REST capability.
type restOnlyTarget struct {
	*target.MemTarget
}

func newRESTOnlyTarget() *restOnlyTarget {
	return &restOnlyTarget{MemTarget: target.NewMemTarget()}
}

func (r *restOnlyTarget) Capabilities() capability.Capability {
	return capability.REST
}

func (r *restOnlyTarget) Do(_ context.Context, _ target.HTTPRequest) (*target.HTTPResponse, error) {
	return nil, errors.New("not implemented in test")
}

// rest.resource step
// -----------------------------------------------------------------------------

func TestRestResource_CreatesWhenAbsent(t *testing.T) {
	var postReceived bool
	var postBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case r.Method == "POST" && r.URL.Path == "/hosts":
			postReceived = true
			postBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    desc = "test host"
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.name == \"test\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"name": "test", "port": 80}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if !postReceived {
		t.Fatal("POST was not executed")
	}
	if rec.CountChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.CountChangedOps())
	}

	var body map[string]any
	if err := json.Unmarshal(postBody, &body); err != nil {
		t.Fatalf("POST body is not valid JSON: %v", err)
	}
	if body["name"] != "test" {
		t.Fatalf("expected name=test, got %v", body["name"])
	}
}

func TestRestResource_NoopWhenSatisfied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/hosts" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name": "test", "port": 80}]`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.name == \"test\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"name": "test", "port": 80}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if rec.CountChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops (satisfied), got %d", rec.CountChangedOps())
	}
}

func TestRestResource_UpdatesOnDrift(t *testing.T) {
	var putPath string
	var putBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id": 42, "name": "test", "port": 8080}]`))
		case r.Method == "PUT":
			putPath = r.URL.Path
			putBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.name == \"test\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    found = rest.request { method = "PUT", path = "/hosts/{id}" }
    bindings = {"id": rest.jq { expr = ".id" }}
    state = {"name": "test", "port": 80}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if rec.CountChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.CountChangedOps())
	}
	if putPath != "/hosts/42" {
		t.Fatalf("expected PUT to /hosts/42, got %s", putPath)
	}

	var body map[string]any
	if err := json.Unmarshal(putBody, &body); err != nil {
		t.Fatalf("PUT body is not valid JSON: %v", err)
	}
	if body["port"] != float64(80) {
		t.Fatalf("expected port=80, got %v", body["port"])
	}
}

func TestRestResource_FoundWithoutState_FiresUnconditionally(t *testing.T) {
	var deleteReceived bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id": 7, "name": "test"}]`))
		case r.Method == "DELETE" && r.URL.Path == "/hosts/7":
			deleteReceived = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.name == \"test\")" }
    }
    found = rest.request { method = "DELETE", path = "/hosts/{id}" }
    bindings = {"id": rest.jq { expr = ".id" }}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if !deleteReceived {
		t.Fatal("DELETE was not executed")
	}
	if rec.CountChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.CountChangedOps())
	}
}

func TestRestResource_MissingOnly_IgnoresFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/hosts" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"name": "test", "port": 9999}]`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.name == \"test\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"name": "test", "port": 80}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if rec.CountChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops (no found handler), got %d", rec.CountChangedOps())
	}
}

func TestRestResource_QueryFails_Aborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.name == \"test\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"name": "test"}
  }
}
`, srv.URL)

	_, err := applyREST(t, cfgStr, srv.URL)
	if err == nil {
		t.Fatal("expected error from query failure")
	}
}

func TestRestResource_CreateFails_Aborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case r.Method == "POST":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("create failed"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.name == \"test\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"name": "test"}
  }
}
`, srv.URL)

	_, err := applyREST(t, cfgStr, srv.URL)
	if err == nil {
		t.Fatal("expected error from create failure")
	}
}

func TestRestResource_IntFloat_Comparison(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/hosts" {
			w.Header().Set("Content-Type", "application/json")
			// JSON numbers are float64, scampi integers are int64.
			_, _ = w.Write([]byte(`[{"name": "test", "port": 80, "ssl": true}]`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.name == \"test\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"name": "test", "port": 80, "ssl": true}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if rec.CountChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops (int/float should match), got %d", rec.CountChangedOps())
	}
}

func TestRestResource_Idempotent_SecondApplyNoop(t *testing.T) {
	var created bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/hosts":
			w.Header().Set("Content-Type", "application/json")
			if created {
				_, _ = w.Write([]byte(`[{"name": "test", "port": 80}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		case r.Method == "POST" && r.URL.Path == "/hosts":
			created = true
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.name == \"test\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"name": "test", "port": 80}
  }
}
`, srv.URL)

	// First apply: creates.
	rec1, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("First apply failed: %v\n%s", err, rec1)
	}
	if rec1.CountChangedOps() != 1 {
		t.Fatalf("first apply: expected 1 changed op, got %d", rec1.CountChangedOps())
	}

	// Second apply: noop.
	rec2, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Second apply failed: %v\n%s", err, rec2)
	}
	if rec2.CountChangedOps() != 0 {
		t.Fatalf("second apply: expected 0 changed ops, got %d", rec2.CountChangedOps())
	}
}

func TestRestResource_ConfigLoads(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = "http://localhost/api" }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    desc = "test resource"
    query = rest.request {
      method = "GET"
      path = "/items"
      check = rest.jq { expr = ".[] | select(.name == \"x\")" }
    }
    missing = rest.request { method = "POST", path = "/items" }
    found = rest.request { method = "PUT", path = "/items/{id}" }
    bindings = {"id": rest.jq { expr = ".id" }}
    state = {"name": "x", "value": 1}
  }
}
`
	cfg := loadConfig(t, cfgStr)
	if len(cfg.Deploy) == 0 {
		t.Fatal("expected at least one deploy block")
	}
}

// ref() tests
// -----------------------------------------------------------------------------

func TestRef_CreatesWithResolvedValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// Cert queries and creation.
		case r.Method == "GET" && r.URL.Path == "/certs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case r.Method == "POST" && r.URL.Path == "/certs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": 42, "domain": "app.example.com"}`))
		// Host queries and creation.
		case r.Method == "GET" && r.URL.Path == "/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		case r.Method == "POST" && r.URL.Path == "/hosts":
			body, _ := io.ReadAll(r.Body)
			var data map[string]any
			if err := json.Unmarshal(body, &data); err != nil {
				t.Fatalf("invalid JSON body: %v", err)
			}
			// cert_id should be resolved from the cert step's output.
			if data["cert_id"] != float64(42) {
				t.Fatalf("expected cert_id=42, got %v (%T)", data["cert_id"], data["cert_id"])
			}
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

let cert = rest.resource {
  query = rest.request {
    method = "GET"
    path = "/certs"
    check = rest.jq { expr = ".[] | select(.domain == \"app.example.com\")" }
  }
  missing = rest.request { method = "POST", path = "/certs" }
  state = {"domain": "app.example.com"}
}

std.deploy(name = "test", targets = [api]) {
  cert
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.domain == \"app.example.com\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"domain": "app.example.com", "cert_id": std.ref(cert, ".id")}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if rec.CountChangedOps() != 2 {
		t.Fatalf("expected 2 changed ops, got %d", rec.CountChangedOps())
	}
}

func TestRef_ResolvesFromQueryResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/certs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id": 9, "domain": "app.example.com"}]`))
		case r.Method == "GET" && r.URL.Path == "/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"domain": "app.example.com", "cert_id": 9}]`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

let cert = rest.resource {
  query = rest.request {
    method = "GET"
    path = "/certs"
    check = rest.jq { expr = ".[] | select(.domain == \"app.example.com\")" }
  }
  missing = rest.request { method = "POST", path = "/certs" }
  state = {"domain": "app.example.com"}
}

std.deploy(name = "test", targets = [api]) {
  cert
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.domain == \"app.example.com\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"domain": "app.example.com", "cert_id": std.ref(cert, ".id")}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if rec.CountChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops (both satisfied), got %d", rec.CountChangedOps())
	}
}

func TestRef_InvalidJQExpr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

let cert = rest.resource {
  query = rest.request {
    method = "GET"
    path = "/certs"
    check = rest.jq { expr = ".[] | select(.domain == \"x\")" }
  }
  missing = rest.request { method = "POST", path = "/certs" }
  state = {"domain": "x"}
}

std.deploy(name = "test", targets = [api]) {
  cert
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.domain == \"x\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"cert_id": std.ref(cert, "[invalid jq")}
  }
}
`, srv.URL)

	_, err := applyREST(t, cfgStr, srv.URL)
	if err == nil {
		t.Fatal("expected error for invalid jq expression in ref()")
	}
}

func TestRef_UpdatesWithResolvedValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/certs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id": 9, "domain": "app.example.com"}]`))
		case r.Method == "GET" && r.URL.Path == "/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id": 5, "domain": "app.example.com", "cert_id": 7, "port": 80}]`))
		case r.Method == "PUT" && r.URL.Path == "/hosts/5":
			body, _ := io.ReadAll(r.Body)
			var data map[string]any
			if err := json.Unmarshal(body, &data); err != nil {
				t.Fatalf("invalid JSON body: %v", err)
			}
			if data["cert_id"] != float64(9) {
				t.Fatalf("expected cert_id=9, got %v", data["cert_id"])
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

let cert = rest.resource {
  query = rest.request {
    method = "GET"
    path = "/certs"
    check = rest.jq { expr = ".[] | select(.domain == \"app.example.com\")" }
  }
  missing = rest.request { method = "POST", path = "/certs" }
  state = {"domain": "app.example.com"}
}

std.deploy(name = "test", targets = [api]) {
  cert
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.domain == \"app.example.com\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    found = rest.request { method = "PUT", path = "/hosts/{id}" }
    bindings = {"id": rest.jq { expr = ".id" }}
    state = {"domain": "app.example.com", "cert_id": std.ref(cert, ".id"), "port": 80}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if rec.CountChangedOps() != 1 {
		t.Fatalf("expected 1 changed op (update), got %d", rec.CountChangedOps())
	}
}

func TestRef_MissingFromStepsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/hosts" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

let cert = rest.resource {
  query = rest.request {
    method = "GET"
    path = "/certs"
    check = rest.jq { expr = ".[] | select(.domain == \"x\")" }
  }
  missing = rest.request { method = "POST", path = "/certs" }
  state = {"domain": "x"}
}

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.domain == \"x\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"domain": "x", "cert_id": std.ref(cert, ".id")}
  }
}
`, srv.URL)

	_, err := applyREST(t, cfgStr, srv.URL)
	if err == nil {
		t.Fatal("expected error when ref target step is not in steps list")
	}
}

func TestRef_NonStepArgument(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = "http://localhost/api" }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.domain == \"x\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"cert_id": std.ref("not-a-step", ".id")}
  }
}
`
	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	_, err := engine.LoadConfig(context.Background(), em, "/config.scampi", store, src)
	if err == nil {
		t.Fatal("expected error when ref() receives non-step argument")
	}
}

func TestRef_ConfigLoads(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = "http://localhost/api" }

let cert = rest.resource {
  query = rest.request {
    method = "GET"
    path = "/certs"
    check = rest.jq { expr = ".[] | select(.domain == \"x\")" }
  }
  missing = rest.request { method = "POST", path = "/certs" }
  state = {"domain": "x"}
}

std.deploy(name = "test", targets = [api]) {
  cert
  rest.resource {
    query = rest.request {
      method = "GET"
      path = "/hosts"
      check = rest.jq { expr = ".[] | select(.domain == \"x\")" }
    }
    missing = rest.request { method = "POST", path = "/hosts" }
    state = {"domain": "x", "cert_id": std.ref(cert, ".id")}
  }
}
`
	cfg := loadConfig(t, cfgStr)
	if len(cfg.Deploy) == 0 {
		t.Fatal("expected at least one deploy block")
	}
}

// rest.resource_set step
// -----------------------------------------------------------------------------

func TestRestResourceSet_CreatesMissingItems(t *testing.T) {
	var posts [][]byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data": []}`))
		case r.Method == "POST" && r.URL.Path == "/users":
			body, _ := io.ReadAll(r.Body)
			posts = append(posts, body)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = [
      {"mac": "aa:bb", "name": "alpha"},
      {"mac": "cc:dd", "name": "beta"},
    ]
    missing = rest.request { method = "POST", path = "/users" }
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 POSTs, got %d", len(posts))
	}
	if rec.CountChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.CountChangedOps())
	}
}

func TestRestResourceSet_NoopWhenConverged(t *testing.T) {
	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.Method == "GET" && r.URL.Path == "/users" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data": [{"mac": "aa:bb", "name": "alpha"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = [{"mac": "aa:bb", "name": "alpha"}]
    missing = rest.request { method = "POST", path = "/users" }
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if rec.CountChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops, got %d", rec.CountChangedOps())
	}
	// Only the query GET should have been made.
	if requestCount.Load() != 1 {
		t.Fatalf("expected 1 request (query only), got %d", requestCount.Load())
	}
}

func TestRestResourceSet_UpdatesOnDrift(t *testing.T) {
	var puts [][]byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data": [{"_id": "123", "mac": "aa:bb", "name": "old-name"}]}`))
		case r.Method == "PUT":
			body, _ := io.ReadAll(r.Body)
			puts = append(puts, body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = [{"mac": "aa:bb", "name": "new-name"}]
    missing = rest.request { method = "POST", path = "/users" }
    found = rest.request { method = "PUT", path = "/users/{id}" }
    bindings = {"id": rest.jq { expr = "._id" }}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if len(puts) != 1 {
		t.Fatalf("expected 1 PUT, got %d", len(puts))
	}

	var body map[string]any
	if err := json.Unmarshal(puts[0], &body); err != nil {
		t.Fatalf("PUT body not valid JSON: %v", err)
	}
	if body["name"] != "new-name" {
		t.Fatalf("expected name=new-name, got %v", body["name"])
	}
}

func TestRestResourceSet_RemovesOrphans(t *testing.T) {
	var orphanPuts [][]byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data": [{"_id": "999", "mac": "zz:zz", "name": "stale", "use_fixedip": true}]}`))
		case r.Method == "PUT":
			body, _ := io.ReadAll(r.Body)
			orphanPuts = append(orphanPuts, body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = []
    orphan = rest.request { method = "PUT", path = "/users/{id}" }
    bindings = {"id": rest.jq { expr = "._id" }}
    orphan_state = {"use_fixedip": false}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if len(orphanPuts) != 1 {
		t.Fatalf("expected 1 orphan PUT, got %d", len(orphanPuts))
	}

	var body map[string]any
	if err := json.Unmarshal(orphanPuts[0], &body); err != nil {
		t.Fatalf("orphan PUT body not valid JSON: %v", err)
	}
	if body["use_fixedip"] != false {
		t.Fatalf("expected use_fixedip=false, got %v", body["use_fixedip"])
	}
}

func TestRestResourceSet_OrphanOmitted_ExtrasIgnored(t *testing.T) {
	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.Method == "GET" && r.URL.Path == "/users" {
			w.Header().Set("Content-Type", "application/json")
			// Remote has an extra item not in declared set.
			_, _ = w.Write([]byte(`{"data": [
				{"mac": "aa:bb", "name": "alpha"},
				{"mac": "zz:zz", "name": "extra"}
			]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = [{"mac": "aa:bb", "name": "alpha"}]
    missing = rest.request { method = "POST", path = "/users" }
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if rec.CountChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops (extra ignored), got %d", rec.CountChangedOps())
	}
}

func TestRestResourceSet_Mixed(t *testing.T) {
	var posts, puts, orphanPuts int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data": [
				{"_id": "1", "mac": "aa:bb", "name": "old-name"},
				{"_id": "2", "mac": "zz:zz", "name": "stale", "use_fixedip": true}
			]}`))
		case r.Method == "POST" && r.URL.Path == "/users":
			posts++
			w.WriteHeader(http.StatusCreated)
		case r.Method == "PUT" && r.URL.Path == "/users/1":
			puts++
			w.WriteHeader(http.StatusOK)
		case r.Method == "PUT" && r.URL.Path == "/users/2":
			orphanPuts++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = [
      {"mac": "aa:bb", "name": "new-name"},
      {"mac": "cc:dd", "name": "fresh"},
    ]
    missing = rest.request { method = "POST", path = "/users" }
    found = rest.request { method = "PUT", path = "/users/{id}" }
    orphan = rest.request { method = "PUT", path = "/users/{id}" }
    bindings = {"id": rest.jq { expr = "._id" }}
    orphan_state = {"use_fixedip": false}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if posts != 1 {
		t.Fatalf("expected 1 POST (missing), got %d", posts)
	}
	if puts != 1 {
		t.Fatalf("expected 1 PUT (drift), got %d", puts)
	}
	if orphanPuts != 1 {
		t.Fatalf("expected 1 orphan PUT, got %d", orphanPuts)
	}
}

func TestRestResourceSet_ConfigLoads(t *testing.T) {
	cfgStr := `
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = "http://localhost" }

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = [{"mac": "aa:bb", "name": "test"}]
    missing = rest.request { method = "POST", path = "/users" }
    found = rest.request { method = "PUT", path = "/users/{id}" }
    orphan = rest.request { method = "PUT", path = "/users/{id}" }
    bindings = {"id": rest.jq { expr = "._id" }}
    orphan_state = {"use_fixedip": false}
  }
}
`
	cfg := loadConfig(t, cfgStr)
	if len(cfg.Deploy) == 0 {
		t.Fatal("expected at least one deploy block")
	}
}

func TestRestResourceSet_StructLiteralItems(t *testing.T) {
	var posts [][]byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data": []}`))
		case r.Method == "POST" && r.URL.Path == "/users":
			body, _ := io.ReadAll(r.Body)
			posts = append(posts, body)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Exercises the real-world pattern: struct literals in a list,
	// indexed with c["field"] in a comprehension to build map items.
	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

let clients = [
  { name = "alpha", mac = "aa:bb" },
  { name = "beta",  mac = "cc:dd" },
]

let items = [{"mac": c["mac"], "name": c["name"]} for c in clients]

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = items
    missing = rest.request { method = "POST", path = "/users" }
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 POSTs, got %d", len(posts))
	}

	var body map[string]any
	if err := json.Unmarshal(posts[0], &body); err != nil {
		t.Fatalf("POST body not valid JSON: %v", err)
	}
	if body["mac"] == nil {
		t.Fatal("mac field is nil -- struct index access broken")
	}
}

func TestRestResourceSet_OrphanFilter_MatchingOrphaned(t *testing.T) {
	var orphanPuts [][]byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/users":
			w.Header().Set("Content-Type", "application/json")
			// Two remote items: one with fixed IP (should be orphaned),
			// one plain DHCP client (should be ignored by filter).
			_, _ = w.Write([]byte(`{"data": [
				{"_id": "1", "mac": "aa:aa", "use_fixedip": true},
				{"_id": "2", "mac": "bb:bb", "use_fixedip": false}
			]}`))
		case r.Method == "PUT":
			body, _ := io.ReadAll(r.Body)
			orphanPuts = append(orphanPuts, body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = []
    orphan = rest.request { method = "PUT", path = "/users/{id}" }
    orphan_filter = rest.jq { expr = "select(.use_fixedip == true)" }
    bindings = {"id": rest.jq { expr = "._id" }}
    orphan_state = {"use_fixedip": false}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	// Only the fixed-IP user should be orphaned, not the DHCP client.
	if len(orphanPuts) != 1 {
		t.Fatalf("expected 1 orphan PUT, got %d", len(orphanPuts))
	}
}

func TestRestResourceSet_OrphanFilter_NoFilterOrphansAll(t *testing.T) {
	var orphanPuts int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data": [
				{"_id": "1", "mac": "aa:aa", "use_fixedip": true},
				{"_id": "2", "mac": "bb:bb", "use_fixedip": false}
			]}`))
		case r.Method == "PUT":
			orphanPuts++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource_set {
    desc = "users"
    query = rest.request {
      method = "GET"
      path = "/users"
      check = rest.jq { expr = ".data[]" }
    }
    key = rest.jq { expr = ".mac" }
    items = []
    orphan = rest.request { method = "PUT", path = "/users/{id}" }
    bindings = {"id": rest.jq { expr = "._id" }}
    orphan_state = {"use_fixedip": false}
  }
}
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}
	// No orphan_filter — both remote items should be orphaned.
	if orphanPuts != 2 {
		t.Fatalf("expected 2 orphan PUTs, got %d", orphanPuts)
	}
}
