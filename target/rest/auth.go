// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"

	"scampi.dev/scampi/errs"
)

// TraceFunc is an optional callback for transport lifecycle events.
type TraceFunc func(msg string)

// AuthConfig produces an http.RoundTripper that layers authentication onto a
// base transport. Implementations are constructed in scampi (rest.basic,
// rest.bearer, rest.header) and stored in Config.Auth.
type AuthConfig interface {
	Transport(base http.RoundTripper, trace TraceFunc) http.RoundTripper
	Kind() string
}

// No Auth
// -----------------------------------------------------------------------------

type NoAuthConfig struct{}

func (NoAuthConfig) Kind() string                                                    { return "none" }
func (NoAuthConfig) Transport(base http.RoundTripper, _ TraceFunc) http.RoundTripper { return base }

// BasicAuth
// -----------------------------------------------------------------------------

type BasicAuthConfig struct {
	User     string
	Password string
}

func (BasicAuthConfig) Kind() string { return "basic" }

func (c BasicAuthConfig) Transport(base http.RoundTripper, trace TraceFunc) http.RoundTripper {
	return &basicTransport{base: base, user: c.User, password: c.Password, trace: trace}
}

type basicTransport struct {
	base     http.RoundTripper
	user     string
	password string
	trace    TraceFunc
	logged   bool
}

func (t *basicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.logged && t.trace != nil {
		t.trace("basic: authenticating as " + t.user)
		t.logged = true
	}
	req = req.Clone(req.Context())
	req.SetBasicAuth(t.user, t.password)
	return t.base.RoundTrip(req)
}

// Header Auth
// -----------------------------------------------------------------------------

type HeaderAuthConfig struct {
	Name  string
	Value string
}

func (HeaderAuthConfig) Kind() string { return "header" }

func (c HeaderAuthConfig) Transport(base http.RoundTripper, trace TraceFunc) http.RoundTripper {
	return &headerTransport{base: base, name: c.Name, value: c.Value, trace: trace}
}

type headerTransport struct {
	base   http.RoundTripper
	name   string
	value  string
	trace  TraceFunc
	logged bool
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.logged && t.trace != nil {
		t.trace("header: using " + t.name)
		t.logged = true
	}
	req = req.Clone(req.Context())
	req.Header.Set(t.name, t.value)
	return t.base.RoundTrip(req)
}

// Bearer Auth (credential exchange)
// -----------------------------------------------------------------------------

type BearerAuthConfig struct {
	TokenEndpoint string // path relative to base URL (e.g. "/tokens")
	Identity      string
	Secret        string
}

func (BearerAuthConfig) Kind() string { return "bearer" }

func (c BearerAuthConfig) Transport(base http.RoundTripper, trace TraceFunc) http.RoundTripper {
	return &bearerTransport{
		base:          base,
		tokenEndpoint: c.TokenEndpoint,
		identity:      c.Identity,
		secret:        c.Secret,
		trace:         trace,
	}
}

type bearerTransport struct {
	base          http.RoundTripper
	tokenEndpoint string // full URL (resolved by RESTTarget.Create)
	identity      string
	secret        string
	trace         TraceFunc

	mu    sync.Mutex
	token string
}

// bare-error: sentinel for bearer auth token fetch failures
var errTokenFetch = errs.New("bearer token fetch")

func (t *bearerTransport) emit(msg string) {
	if t.trace != nil {
		t.trace(msg)
	}
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Skip auth for the token endpoint itself to avoid recursion.
	reqURL := req.URL.String()
	if reqURL == t.tokenEndpoint {
		return t.base.RoundTrip(req)
	}

	if err := t.ensureToken(req); err != nil {
		return nil, err
	}

	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// On 401, re-authenticate once and retry.
	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		t.emit("bearer: 401 received, re-authenticating")
		t.mu.Lock()
		t.token = ""
		t.mu.Unlock()

		if err := t.ensureToken(req); err != nil {
			return nil, err
		}
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.token)
		return t.base.RoundTrip(req)
	}

	return resp, nil
}

func (t *bearerTransport) ensureToken(origReq *http.Request) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token != "" {
		t.emit("bearer: cached token valid")
		return nil
	}

	t.emit("bearer: fetching token from " + t.tokenEndpoint)
	body, err := json.Marshal(map[string]string{
		"identity": t.identity,
		"secret":   t.secret,
	})
	if err != nil {
		return errs.WrapErrf(errTokenFetch, "marshal credentials: %v", err)
	}

	tokenReq, err := http.NewRequestWithContext(
		origReq.Context(),
		http.MethodPost,
		t.tokenEndpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return errs.WrapErrf(errTokenFetch, "build request: %v", err)
	}
	tokenReq.Header.Set("Content-Type", "application/json")

	resp, err := t.base.RoundTrip(tokenReq)
	if err != nil {
		return errs.WrapErrf(errTokenFetch, "%v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Body intentionally omitted — some token servers echo the
		// submitted credentials back into 4xx bodies (#312). Status
		// and endpoint URL is enough to debug.
		return errs.WrapErrf(errTokenFetch, "status %d from %s", resp.StatusCode, t.tokenEndpoint)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return errs.WrapErrf(errTokenFetch, "decode response: %v", err)
	}

	tok, ok := result["token"]
	if !ok {
		// Fall back to OAuth2-style "access_token" field.
		tok, ok = result["access_token"]
	}
	if !ok {
		return errs.WrapErrf(errTokenFetch, "response contains neither \"token\" nor \"access_token\" field")
	}

	tokenStr, ok := tok.(string)
	if !ok {
		return errs.WrapErrf(errTokenFetch, "token field is %T, expected string", tok)
	}

	t.token = tokenStr
	t.emit("bearer: token acquired")
	return nil
}
