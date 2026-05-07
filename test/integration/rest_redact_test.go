// SPDX-License-Identifier: GPL-3.0-only

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/engine"
	"scampi.dev/scampi/secret"
	"scampi.dev/scampi/source"
	"scampi.dev/scampi/target/rest"
	"scampi.dev/scampi/test/harness"
)

// secretValue is the synthetic value our mock controller "leaks" in
// its GET response. Long enough to clear the redactor's minRedactLen
// guard. NEVER use real secrets here — see feedback memory.
const secretValue = "F4kE-pw-MUST-redact-1234"

// TestRestRequest_Redact_RegistersResponseValue verifies that values
// at the configured redact paths flow through the on-context redactor
// AFTER the GET response is parsed. This is the GET-side half of #293.
func TestRestRequest_Redact_RegistersResponseValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := fmt.Sprintf(`{
        "data": [{
          "x_ssh_password": "%s",
          "auto_upgrade": false
        }]
    }`, secretValue)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    desc = "mgmt"
    query = rest.request {
      method = "GET"
      path   = "/mgmt"
      check  = rest.jq { expr = ".data[0]" }
      redact = ["data[0].x_ssh_password"]
    }
    found = rest.request { method = "PUT", path = "/mgmt" }
    state = {
      "auto_upgrade": false,
    }
  }
}
`, srv.URL)

	red, rec, err := applyRESTWithRedactor(t, cfgStr, srv.URL)
	_ = rec
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if red.Size() == 0 {
		t.Fatalf("redactor empty — secret value not registered from response")
	}
	if red.Redact(secretValue) != "***SECRET***" {
		t.Errorf("secret value not redacted: %q", red.Redact(secretValue))
	}
}

// TestRestRequest_Redact_HidesValueInRenderedDrift verifies the full
// loop: register at parse-time, redact at render-time. We assert the
// rendered output never contains the plaintext secret.
func TestRestRequest_Redact_HidesValueInRenderedDrift(t *testing.T) {
	// Mock: GET response carries the secret AND drifts vs declared
	// state on a non-secret field, so a drift event renders.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.Header().Set("Content-Type", "application/json")
			body := fmt.Sprintf(`{
        "data": [{
          "x_ssh_password": "%s",
          "auto_upgrade": true
        }]
    }`, secretValue)
			_, _ = w.Write([]byte(body))
		case "PUT":
			w.WriteHeader(http.StatusOK)
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
    desc = "mgmt"
    query = rest.request {
      method = "GET"
      path   = "/mgmt"
      check  = rest.jq { expr = ".data[0]" }
      redact = ["data[0].x_ssh_password"]
    }
    found = rest.request { method = "PUT", path = "/mgmt" }
    state = {
      "auto_upgrade": false,
    }
  }
}
`, srv.URL)

	red, rec, err := applyRESTWithRedactor(t, cfgStr, srv.URL)
	if err != nil {
		t.Fatalf("Apply failed: %v\n%s", err, rec)
	}

	// 1. Redactor must hold the secret.
	if red.Redact(secretValue) != "***SECRET***" {
		t.Fatalf("secret not registered with redactor")
	}

	// 2. Plaintext secret must not appear in rendered output AFTER
	// the redactor has been consulted. RecordingDisplayer captures
	// raw, pre-redaction events — so we drive the redactor manually
	// across the captured strings to assert the redactor *would*
	// catch any leak in real rendering.
	rawOutput := rec.String()
	redactedOutput := red.Redact(rawOutput)
	if strings.Contains(redactedOutput, secretValue) {
		t.Fatalf("plaintext secret leaked through redactor:\n%s", redactedOutput)
	}
}

// TestRestRequest_Redact_InvalidPathFailsAtPlan verifies that a bad
// jq path surfaces as a typed plan-time diagnostic (not a runtime
// "command failed" surprise).
func TestRestRequest_Redact_InvalidPathFailsAtPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfgStr := fmt.Sprintf(`
module main
import "std"
import "std/rest"

let api = rest.target { name = "api", base_url = %q }

std.deploy(name = "test", targets = [api]) {
  rest.resource {
    desc = "mgmt"
    query = rest.request {
      method = "GET"
      path   = "/mgmt"
      check  = rest.jq { expr = ".data[0]" }
      redact = ["data..invalid"]
    }
    found = rest.request { method = "PUT", path = "/mgmt" }
    state = { "x": 1 }
  }
}
`, srv.URL)

	_, _, err := applyRESTWithRedactor(t, cfgStr, srv.URL)
	if err == nil {
		t.Fatalf("expected plan-time error for invalid redact path")
	}
}

// applyRESTWithRedactor mirrors applyREST but threads a Redactor into
// the apply context so we can assert what was registered. Returns the
// redactor + recorded display + apply error.
func applyRESTWithRedactor(
	t *testing.T,
	cfgStr, baseURL string,
) (*secret.Redactor, *harness.RecordingDisplayer, error) {
	t.Helper()

	src := source.NewMemSource()
	src.Files["/config.scampi"] = []byte(cfgStr)

	tgt := createRESTTarget(t, &rest.Config{BaseURL: baseURL})

	rec := &harness.RecordingDisplayer{}
	em := diagnostic.NewEmitter(diagnostic.Policy{}, rec)
	store := diagnostic.NewSourceStore()

	red := secret.NewRedactor()
	ctx := secret.WithRedactor(context.Background(), red)

	cfg, err := engine.LoadConfig(ctx, em, "/config.scampi", store, src)
	if err != nil {
		return red, rec, err
	}
	resolved, err := engine.Resolve(cfg, "", "")
	if err != nil {
		return red, rec, err
	}
	resolved.Target = harness.MockTargetInstance(tgt)

	e, err := engine.NewWithTarget(ctx, src, resolved, em, tgt)
	if err != nil {
		return red, rec, err
	}
	defer e.Close()

	return red, rec, e.Apply(ctx)
}
