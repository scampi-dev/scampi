---
title: resource
---

Declarative REST resource management. Composes
[`rest.request`]({{< relref "request" >}}) primitives into a single
convergence-aware step that queries for a resource and reacts based on
whether it was found or missing.

```scampi
rest.resource {
  desc  = "app.example.com proxy host"
  query = rest.request {
    method = "GET"
    path   = "/nginx/proxy-hosts"
    check  = rest.jq { expr = ".[] | select(.domain_names[0] == \"app.example.com\")" }
  }
  missing = rest.request {
    method = "POST"
    path   = "/nginx/proxy-hosts"
  }
  found = rest.request {
    method = "PUT"
    path   = "/nginx/proxy-hosts/{id}"
  }
  bindings = {
    "id": rest.jq { expr = ".id" },
  }
  state = {
    "domain_names":  ["app.example.com"],
    "forward_host":  "198.51.100.80",
    "forward_port":  2001,
    "certificate_id": 9,
    "ssl_forced":    true,
  }
}
```

## Fields

| Field       | Type                      | Required | Description                                 |
| ----------- | ------------------------- | :------: | ------------------------------------------- |
| `query`     | `rest.request?`           |          | Request to check if the resource exists     |
| `missing`   | `rest.request?`           |          | Request to execute when resource is missing |
| `found`     | `rest.request?`           |          | Request to execute when resource is found   |
| `state`     | map\[string, any]?        |          | Desired resource state (dict)               |
| `bindings`  | map\[string, rest.Check]? |          | Name-to-jq mappings for path interpolation  |
| `desc`      | string?                   |          | Human-readable description                  |
| `on_change` | list\[Step]               |          | Steps to trigger when the resource changes  |

At least one of `missing` or `found` is required.

## Query

The `query` must be a `rest.request` with a `rest.jq` check. The jq
expression filters the API response to find the target resource:

- If the expression matches nothing, the resource is **missing** and the
  `missing` request fires.
- If the expression matches an object, the resource is **found**. That
  object becomes available for drift detection and binding resolution.

The query check must use `rest.jq`, not `rest.status`. The step needs the
matched object, not just a status code.

## State and drift detection

The optional `state` dict declares the desired resource properties. When both
`state` and `found` are set, each key in `state` is compared against the
corresponding key in the query result during check. Only keys present in
`state` are compared — extra fields in the API response are ignored.

- All keys match → **noop** (already converged)
- Any key differs → **found** fires with `state` as the JSON request body

When `state` is set and `missing` fires, `state` is used as the JSON body
for the create request.

When `found` is set without `state`, the `found` request fires
unconditionally whenever the query matches — useful for delete-if-exists
patterns:

```scampi
rest.resource {
  query = rest.request {
    method = "GET"
    path   = "/nginx/proxy-hosts"
    check  = rest.jq { expr = ".[] | select(.domain_names[0] == \"stale.example.com\")" }
  }
  found    = rest.request { method = "DELETE", path = "/nginx/proxy-hosts/{id}" }
  bindings = {"id": rest.jq { expr = ".id" }}
}
```

## Bindings

Bindings extract values from the query result for use in the `found`
request path. Each binding maps a name to a `rest.jq` expression that
runs against the matched query result object.

```scampi
bindings = {
  "id":      rest.jq { expr = ".id" },
  "version": rest.jq { expr = ".meta.version" },
}
```

Resolved binding values replace `{name}` placeholders in the `found` path:

```scampi
found = rest.request { method = "PUT", path = "/hosts/{id}?v={version}" }
```

Bindings only apply to the `found` request path and require `found` to be
configured.

## Execution flow

1. **Check** — run query (GET by default), apply jq filter
2. No match → resource missing → **missing** fires (with `state` as body
   if set)
3. Match found, no `found` handler → **noop**
4. Match found, `found` handler, no `state` → **found** fires
   unconditionally
5. Match found, `found` handler, `state` set → diff against `state` → no
   drift: **noop**, drift: **found** fires (with `state` as body)

## Error handling

- Query failure (network error, non-2xx status) aborts the step. No
  fallthrough to `missing` or `found`.
- `missing` or `found` failure (status >= 400) aborts the step.

There are no error-suppression flags. For fire-and-forget HTTP calls, use a
bare [`rest.request`]({{< relref "request" >}}) instead.
