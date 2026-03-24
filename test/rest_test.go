// SPDX-License-Identifier: GPL-3.0-only

package test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
)

// Config loading
// -----------------------------------------------------------------------------

func TestRESTTarget_ConfigLoads(t *testing.T) {
	cfgStr := `
target.rest(name="api", base_url="http://localhost:8080/api")
deploy(name="test", targets=["api"], steps=[run(apply="echo ok", check="true")])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	cfg, err := engine.LoadConfig(context.Background(), em, "/config.star", store, src)
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
target.rest(
    name="api",
    base_url="http://localhost/api",
    auth=rest.basic(user="admin", password="secret"),
)
deploy(name="test", targets=["api"], steps=[run(apply="echo ok", check="true")])
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
target.rest(
    name="api",
    base_url="http://localhost/api",
    auth=rest.bearer(
        token_endpoint="/tokens",
        identity="admin@test.com",
        secret="pass123",
    ),
)
deploy(name="test", targets=["api"], steps=[run(apply="echo ok", check="true")])
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
target.rest(
    name="api",
    base_url="http://localhost/api",
    auth=rest.header(name="X-API-Key", value="my-key"),
)
deploy(name="test", targets=["api"], steps=[run(apply="echo ok", check="true")])
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
target.rest(name="api", base_url="https://localhost/api", tls=rest.tls.insecure())
deploy(name="test", targets=["api"], steps=[run(apply="echo ok", check="true")])
`
	cfg := loadConfig(t, cfgStr)
	restCfg := cfg.Targets["api"].Config.(*rest.Config)

	if _, ok := restCfg.TLS.(rest.InsecureTLSConfig); !ok {
		t.Fatalf("expected InsecureTLSConfig, got %T", restCfg.TLS)
	}
}

func TestRESTTarget_ConfigWithTLSSecure(t *testing.T) {
	cfgStr := `
target.rest(name="api", base_url="https://localhost/api", tls=rest.tls.secure())
deploy(name="test", targets=["api"], steps=[run(apply="echo ok", check="true")])
`
	cfg := loadConfig(t, cfgStr)
	restCfg := cfg.Targets["api"].Config.(*rest.Config)

	if _, ok := restCfg.TLS.(rest.SecureTLSConfig); !ok {
		t.Fatalf("expected SecureTLSConfig, got %T", restCfg.TLS)
	}
}

func TestRESTTarget_ConfigInvalidAuth(t *testing.T) {
	cfgStr := `
target.rest(name="api", base_url="http://localhost/api", auth="not-an-auth")
deploy(name="test", targets=["api"], steps=[run(apply="echo ok", check="true")])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	_, err := engine.LoadConfig(context.Background(), em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for invalid auth type")
	}
}

func TestRESTTarget_ConfigEmptyName(t *testing.T) {
	cfgStr := `
target.rest(name="", base_url="http://localhost/api")
deploy(name="test", targets=["api"], steps=[run(apply="echo ok", check="true")])
`
	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	_, err := engine.LoadConfig(context.Background(), em, "/config.star", store, src)
	if err == nil {
		t.Fatal("expected error for empty target name")
	}
}

// Capability mismatch
// -----------------------------------------------------------------------------

func TestRESTTarget_RejectsPOSIXSteps(t *testing.T) {
	cfgStr := `
target.local(name="local")
deploy(
    name="test",
    targets=["local"],
    steps=[copy(src=local("/a"), dest="/b", perm="0644", owner="user", group="group")],
)
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
target.rest(name="api", base_url=%q)
deploy(name="test", targets=["api"], steps=[
    rest.request(
        desc="create item",
        method="POST",
        path="/items",
        body=rest.body.json({"name": "test"}),
        check=rest.status(code=200),
    ),
])
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if !created {
		t.Fatal("POST was not executed")
	}
	if rec.countChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.countChangedOps())
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
target.rest(name="api", base_url=%q)
deploy(name="test", targets=["api"], steps=[
    rest.request(
        desc="create item",
        method="POST",
        path="/items",
        check=rest.status(code=200),
    ),
])
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if rec.countChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops (already satisfied), got %d", rec.countChangedOps())
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
target.rest(name="api", base_url=%q)
deploy(name="test", targets=["api"], steps=[
    rest.request(
        desc="create host",
        method="POST",
        path="/hosts",
        body=rest.body.json({"domain": "example.com"}),
        check=rest.jq(expr='.[] | select(.domain == "example.com")'),
    ),
])
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if rec.countChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.countChangedOps())
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
target.rest(name="api", base_url=%q)
deploy(name="test", targets=["api"], steps=[
    rest.request(
        desc="create host",
        method="POST",
        path="/hosts",
        check=rest.jq(expr='.[] | select(.domain == "example.com")'),
    ),
])
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if rec.countChangedOps() != 0 {
		t.Fatalf("expected 0 changed ops (already satisfied), got %d", rec.countChangedOps())
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
target.rest(name="api", base_url=%q)
deploy(name="test", targets=["api"], steps=[
    rest.request(
        desc="update config",
        method="PUT",
        path="/config",
        body=rest.body.json({"key": "value"}),
    ),
])
`, srv.URL)

	rec, err := applyREST(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	if !called {
		t.Fatal("PUT was not executed")
	}
	if rec.countChangedOps() != 1 {
		t.Fatalf("expected 1 changed op, got %d", rec.countChangedOps())
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
target.rest(name="api", base_url=%q)
deploy(name="test", targets=["api"], steps=[
    rest.request(
        method="POST",
        path="/items",
        check=rest.status(code=200),
    ),
])
`, srv.URL)

	_, err := applyREST(t, cfgStr, srv.URL)
	if err == nil {
		t.Fatal("expected error from 500 response")
	}
}

func applyREST(t *testing.T, cfgStr, baseURL string) (*recordingDisplayer, error) {
	t.Helper()

	src := source.NewMemSource()
	src.Files["/config.star"] = []byte(cfgStr)

	tgt := createRESTTarget(t, &rest.Config{BaseURL: baseURL})

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	ctx := context.Background()
	cfg, err := engine.LoadConfig(ctx, em, "/config.star", store, src)
	if err != nil {
		return rec, err
	}

	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		return rec, err
	}

	resolved.Target = mockTargetInstance(tgt)

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
	src.Files["/config.star"] = []byte(cfgStr)

	rec := &recordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	cfg, err := engine.LoadConfig(context.Background(), em, "/config.star", store, src)
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
