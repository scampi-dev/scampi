// SPDX-License-Identifier: GPL-3.0-only

package rest_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/spec"
	"scampi.dev/scampi/target"
	"scampi.dev/scampi/target/rest"
)

// Regression test for #312. Both identity and secret are long enough to
// clear the redactor's minRedactLen guard. Synthetic — never paste real
// credentials into tests.
const (
	bearerIdentity = "test-bearer-identity-AAAAAAAAAA"
	bearerSecret   = "test-bearer-secret-ZZZZZZZZZZZZ"
)

// Bearer token-fetch failures must NOT inline the response body — some
// upstream servers helpfully echo the submitted credentials back into
// 4xx bodies (#312). Identity and secret must also be registered with
// the on-context redactor as defense in depth.
func TestBearerTokenFetch_4xx_OmitsResponseBodyAndRedacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		// Simulate an upstream that echoes credentials in its error body.
		_, _ = fmt.Fprintf(w, "rejected: identity=%s secret=%s", bearerIdentity, bearerSecret)
	}))
	defer srv.Close()

	red := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), red)

	cfg := &rest.Config{
		BaseURL: srv.URL,
		Auth: rest.BearerAuthConfig{
			TokenEndpoint: "/token",
			Identity:      bearerIdentity,
			Secret:        bearerSecret,
		},
		TLS: rest.SecureTLSConfig{},
	}
	inst := spec.TargetInstance{Config: cfg}
	tgt, err := rest.REST{}.Create(ctx, nil, inst)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	restTgt, ok := tgt.(*rest.RESTTarget)
	if !ok {
		t.Fatalf("expected *rest.RESTTarget, got %T", tgt)
	}
	defer restTgt.Close()

	_, err = restTgt.Do(ctx, target.HTTPRequest{Method: "GET", Path: "/anything"})
	if err == nil {
		t.Fatal("expected error from token fetch returning 401, got nil")
	}

	rawMsg := err.Error()
	if strings.Contains(rawMsg, "rejected:") {
		t.Errorf("error wrapped the response body verbatim:\n%s", rawMsg)
	}
	if strings.Contains(rawMsg, bearerIdentity) {
		t.Errorf("raw error contains identity verbatim:\n%s", rawMsg)
	}
	if strings.Contains(rawMsg, bearerSecret) {
		t.Errorf("raw error contains secret verbatim:\n%s", rawMsg)
	}

	// Defense in depth: even if a future code path slips creds into a
	// message, the redactor must scrub them.
	redacted := red.Redact("identity=" + bearerIdentity + " secret=" + bearerSecret)
	if strings.Contains(redacted, bearerIdentity) {
		t.Errorf("redactor did not scrub identity: %q", redacted)
	}
	if strings.Contains(redacted, bearerSecret) {
		t.Errorf("redactor did not scrub secret: %q", redacted)
	}
}
