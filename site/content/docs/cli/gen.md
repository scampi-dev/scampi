---
title: gen
weight: 5
---

Generate Starlark modules from external schemas. Each generator produces typed
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

| Flag             | Description                                        |
| ---------------- | -------------------------------------------------- |
| `-o`, `--output` | Output file path (default: derives from spec name) |
| `-p`, `--prefix` | Path prefix prepended to all generated routes      |

By default the output file is named after the spec: `npm-openapi.yaml` produces
`npm-openapi.api.scampi`. Use `-o -` to write to stdout.

When the API is served behind a proxy path, use `--prefix` to prepend to
all generated routes:

```text
scampi gen api --prefix=/integration unifi-network.json
```

This generates `path = "/integration/v1/sites/" + siteId + "/networks"` instead
of `path = "/v1/sites/" + siteId + "/networks"`.

### What it generates

Each endpoint in the spec becomes a Starlark function wrapping `rest.request()`
with the correct HTTP method, path, and parameters.

- Path parameters become positional arguments and are interpolated into the path
- Body fields default to `None` — when no body args are passed, the function
  returns a bare `rest.request(method, path)` suitable for use as a
  `rest.resource` template
- When body args are provided, they're collected into a JSON body
- GET endpoints include a `check` parameter for response validation
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

```text
scampi gen api npm-openapi.yaml
```

Produces:

```starlark {filename="npm-openapi.api.scampi"}
// Generated from npm-openapi.yaml by scampi gen api
//
// Nginx Proxy Manager API (subset) 1.0.0
//
// This file was mechanically generated from an OpenAPI specification.
// It is provided as-is with no warranty. Scampi's license does not
// apply to generated output. If the source specification carries its
// own license terms, those terms govern this file.
//
// Usage: load("npm-openapi.api.scampi", ...)

// Certificates
// -----------------------------------------------------------------------------

// GET /nginx/certificates — List all certificates
def get_certificates(check = None):
    return rest.request(
        method = "GET",
        path = "/nginx/certificates",
        check = check,
    )

// POST /nginx/certificates — Create a new certificate
def create_certificate(
        domain_names = None,
        provider = None,
        meta = None):
    body = {}
    if domain_names != None:
        body["domain_names"] = domain_names
    if provider != None:
        body["provider"] = provider
    if meta != None:
        body["meta"] = meta
    if body:
        return rest.request(
            method = "POST",
            path = "/nginx/certificates",
            body = rest.body.json(body),
        )
    return rest.request(
        method = "POST",
        path = "/nginx/certificates",
    )


// Proxy Hosts
// -----------------------------------------------------------------------------

// GET /nginx/proxy-hosts — List all proxy hosts
def get_proxy_hosts(check = None):
    return rest.request(
        method = "GET",
        path = "/nginx/proxy-hosts",
        check = check,
    )

// ... more functions for create, update, delete ...
```

You then write a thin wrapper module that adds convergence semantics.
The generated wrappers work directly as `rest.resource` templates — when called
without body args they return a bare method+path request, and `rest.resource`
provides the body via `state`:

```starlark {filename="npm.scampi"}
load("npm-openapi.api.scampi", "get_certificates", "create_certificate")

def certificate(domain_names, provider = "letsencrypt"):
    return rest.resource(
        query = get_certificates(
            check = rest.jq(
                expr = '.[] | select(.domain_names[0] == "' + domain_names[0] + '")',
            ),
        ),
        missing = create_certificate(),
        state = {
            "domain_names": domain_names,
            "provider": provider,
        },
    )
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
