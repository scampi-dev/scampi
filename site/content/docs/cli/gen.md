---
title: gen
weight: 5
---

Generate scampi modules from external schemas. Each generator produces typed
request wrappers that handle the boring parts (field names, HTTP plumbing) so you
can focus on convergence logic.

## Three-layer pattern

Generated code follows a consistent layering:

1. **`foo.api.scampi`** (generated) — raw typed request wrappers
2. **`foo.scampi`** (user-authored) — convergence-aware resource functions using `rest.resource`
3. **`deploy.scampi`** (user config) — declarative infrastructure

The generator handles layer 1. You write layers 2 and 3.

## api

```text
scampi gen api [flags] <spec.yaml>
```

Generate a `.api.scampi` module from an OpenAPI specification. Supports both
OpenAPI 3.x and Swagger 2.0 specs.

| Flag             | Description                                               |
| ---------------- | --------------------------------------------------------- |
| `-o`, `--output` | Output file path (default: derives from spec name)        |
| `-p`, `--prefix` | Path prefix prepended to all generated routes             |
| `-m`, `--module` | Override the module declaration name (default: spec name) |
| `--no-test`      | Skip generating the companion `*_test.scampi` file        |

By default the output file is named after the spec: `npm-openapi.yaml` produces
`npm-openapi.api.scampi`. Use `-o -` to write to stdout (suppresses test
generation).

A companion smoke test (`*_test.scampi`) is generated alongside the module by
default. It exercises every endpoint against a `test.target_rest_mock` and
verifies the expected requests were made. Pass `--no-test` to skip.

When the API is served behind a proxy path, use `--prefix` to prepend to
all generated routes:

```bash
scampi gen api --prefix=/integration unifi-network.json
```

This generates `path = "/integration/v1/sites/" + siteId + "/networks"` instead
of `path = "/v1/sites/" + siteId + "/networks"`.

### What it generates

Each endpoint in the spec becomes a scampi function wrapping `rest.request`
with the correct HTTP method, path, and parameters.

- Path parameters become typed `string` arguments
- Body fields default to `none` — when no body args are passed, the function
  sends an empty JSON body (`rest.body_json { data = {} }`)
- When body args are provided, they're collected into a JSON body
- GET endpoints include a `check: rest.Check?` parameter for response validation
- Endpoints are grouped by path prefix with section headers

### Example

Given an OpenAPI spec for Nginx Proxy Manager:

```yaml {filename="npm-openapi.yaml"}
openapi: "3.0.3"
info:
  title: Nginx Proxy Manager API (subset)
  version: "1.0.0"

paths:
  /nginx/certificates:
    get:
      operationId: getCertificates
      summary: List all certificates
      responses:
        "200":
          description: Array of certificates
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/Certificate"
    post:
      operationId: createCertificate
      summary: Create a new certificate
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CertificateCreate"
      responses:
        "201":
          description: Created certificate

  /nginx/proxy-hosts:
    # ... more endpoints ...

components:
  schemas:
    CertificateCreate:
      type: object
      required: [domain_names, provider]
      properties:
        domain_names:
          type: array
          items:
            type: string
        provider:
          type: string
        meta:
          type: object
    # ... more schemas ...
```

Running the generator:

```bash
scampi gen api npm-openapi.yaml
```

Produces two files:

```scampi {filename="npm-openapi.api.scampi"}
// Generated from npm-openapi.yaml by scampi gen api
//
// Nginx Proxy Manager API (subset) 1.0.0

module npm_openapi

import "std/rest"

// Certificates
// -----------------------------------------------------------------------------

// GET /nginx/certificates — List all certificates
func get_certificates(check: rest.Check? = none) std.Step {
  return rest.request {
    method = "GET"
    path   = "/nginx/certificates"
    check  = check
  }
}

// POST /nginx/certificates — Create a new certificate
func create_certificate(
  domain_names: string? = none,
  meta: string? = none,
  provider: string? = none
) std.Step {
  let body = {}
  if domain_names != none {
    body["domain_names"] = domain_names
  }
  if meta != none {
    body["meta"] = meta
  }
  if provider != none {
    body["provider"] = provider
  }
  return rest.request {
    method = "POST"
    path   = "/nginx/certificates"
    body   = rest.body_json { data = body }
  }
}


// Proxy Hosts
// -----------------------------------------------------------------------------

// GET /nginx/proxy-hosts — List all proxy hosts
func get_proxy_hosts(check: rest.Check? = none) std.Step {
  return rest.request {
    method = "GET"
    path   = "/nginx/proxy-hosts"
    check  = check
  }
}

// ... more functions for create, update, delete ...
```

And a companion smoke test:

```scampi {filename="npm-openapi_test.scampi"}
// Auto-generated smoke test
// Verifies each endpoint sends the expected method and path.

module main

import "std"
import "std/rest"
import "std/test"
import "std/test/matchers"

let api = test.target_rest_mock(
  name = "api",
  base_url = "http://localhost",
  routes = {
    "GET /nginx/certificates": test.response(status = 200),
    "POST /nginx/certificates": test.response(status = 201),
    "GET /nginx/proxy-hosts": test.response(status = 200),
  },
  expect_requests = [
    test.request(method = "GET", path = "/nginx/certificates"),
    test.request(
      method = "POST",
      path   = "/nginx/certificates",
      body   = matchers.has_substring("\"domain_names\""),
    ),
    test.request(method = "GET", path = "/nginx/proxy-hosts"),
  ],
)

std.deploy(name = "smoke", targets = [api]) {
  rest.request {
    method = "GET"
    path   = "/nginx/certificates"
    check  = rest.status { code = 200 }
  }
  rest.request {
    method = "POST"
    path   = "/nginx/certificates"
    body   = rest.body_json { data = { "domain_names": "test" } }
  }
  rest.request {
    method = "GET"
    path   = "/nginx/proxy-hosts"
    check  = rest.status { code = 200 }
  }
}
```

Run the smoke test with:

```bash
scampi test npm-openapi_test.scampi
```

You then write a thin wrapper module that adds convergence semantics.
The generated wrappers work directly as `rest.resource` templates — when called
without body args they return a bare method+path request, and `rest.resource`
provides the body via `state`:

```scampi {filename="npm.scampi"}
module npm

import "std/rest"

func certificate(
  domain_names: list[string],
  provider: string = "letsencrypt",
) std.Step {
  return rest.resource {
    query = npm_openapi.get_certificates(
      check = rest.jq { expr = ".[] | select(.domain_names[0] == \"" + domain_names[0] + "\")" },
    )
    missing = npm_openapi.create_certificate()
    state = {
      "domain_names": domain_names,
      "provider": provider,
    }
  }
}
```

### Regeneration

Output is idempotent — running the generator twice with the same input produces
identical output. Generated files include a header comment identifying the source
spec, and are meant to be committed to version control so they're auditable and
diffable.

### Swagger 2.0

Swagger 2.0 specs are automatically detected and converted to OpenAPI 3.x before
generation. Both JSON and YAML formats are supported.

## Future generators

The `gen` subcommand is a namespace for additional generators:

- `scampi gen db` — database schemas (planned)
- `scampi gen graphql` — GraphQL schemas (planned)

Each generator follows the same principle: generate the typed plumbing, let the
user add convergence semantics.
