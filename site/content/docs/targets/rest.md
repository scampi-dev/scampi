---
title: rest
---

Make HTTP requests against a REST API. Used for configuring applications that
expose a REST interface — reverse proxies, monitoring tools, container
orchestrators, DNS providers.

```scampi
import "std/rest"

let npm_api = rest.target {
  name     = "npm_api"
  base_url = "http://198.51.100.30:81/api"
  auth     = rest.bearer {
    token_endpoint = "/tokens"
    identity       = std.secret("npm.admin.email")
    secret         = std.secret("npm.admin.password")
  }
}
```

## Fields

| Field      | Type         | Required | Default             | Description                             |
| ---------- | ------------ | :------: | ------------------- | --------------------------------------- |
| `name`     | string       |    ✓     |                     | Identifier for deploy blocks            |
| `base_url` | string       |    ✓     |                     | Base URL prepended to all request paths |
| `auth`     | `rest.Auth?` |          | `rest.no_auth{}`    | Authentication strategy (see below)     |
| `tls`      | `rest.TLS?`  |          | `rest.tls_secure{}` | TLS configuration (see below)           |

## Authentication

Auth strategies are composable — each one wraps the HTTP transport and handles
credentials transparently.

### rest.no_auth

No authentication. Requests are sent without any credentials. This is the
default.

```scampi
auth = rest.no_auth {}
```

### rest.basic

HTTP Basic authentication.

```scampi
auth = rest.basic { user = "admin", password = std.secret("pass") }
```

| Field      | Type   | Required | Description |
| ---------- | ------ | :------: | ----------- |
| `user`     | string |    ✓     | Username    |
| `password` | string |    ✓     | Password    |

### rest.header

Static header authentication. Works for API keys, static bearer tokens, or any
auth that uses a single header.

```scampi
auth = rest.header { name = "X-API-Key", value = std.secret("grafana.api_key") }
```

| Field   | Type   | Required | Description  |
| ------- | ------ | :------: | ------------ |
| `name`  | string |    ✓     | Header name  |
| `value` | string |    ✓     | Header value |

### rest.bearer

Credential exchange. POSTs identity and secret to a token endpoint, caches the
token, and automatically re-authenticates on 401 responses.

```scampi
auth = rest.bearer {
  token_endpoint = "/tokens"
  identity       = std.secret("npm.admin.email")
  secret         = std.secret("npm.admin.password")
}
```

| Field            | Type   | Required | Description                                   |
| ---------------- | ------ | :------: | --------------------------------------------- |
| `token_endpoint` | string |    ✓     | Path to the token endpoint (relative to base) |
| `identity`       | string |    ✓     | Identity/username for credential exchange     |
| `secret`         | string |    ✓     | Secret/password for credential exchange       |

The token endpoint must return JSON with a `token` or `access_token` field.

## TLS

TLS strategies are composable, same as auth — one slot, pick the right one.

### rest.tls_secure

Validate certificates against the system CA pool. This is the default.

```scampi
tls = rest.tls_secure {}
```

### rest.tls_insecure

Skip all certificate verification. Use for testing only.

```scampi
tls = rest.tls_insecure {}
```

### rest.tls_ca_cert

Validate against a custom CA certificate. Use for self-signed or internal CAs.

```scampi
tls = rest.tls_ca_cert { path = "./certs/internal-ca.pem" }
```

| Field  | Type   | Required | Description                        |
| ------ | ------ | :------: | ---------------------------------- |
| `path` | string |    ✓     | Path to PEM-encoded CA certificate |

## How it works

The REST target wraps a standard Go HTTP client. Authentication and TLS are
implemented as composable configuration — each strategy plugs into a single
slot without conflicting fields.

Unlike local and SSH targets, the REST target does not support filesystem,
package, or service operations. Steps that require those capabilities (like
`copy`, `pkg`, or `service`) will fail at plan time with a capability mismatch
error.
