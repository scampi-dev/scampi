---
title: request
---

Make HTTP requests against a REST target with optional idempotency checks.
See the [rest module overview](../) for check matchers and general concepts.

```scampi
rest.request {
  desc   = "create proxy host"
  method = "POST"
  path   = "/nginx/proxy-hosts"
  body   = rest.body_json { data = {"domain_names": ["example.com"], "forward_host": "198.51.100.5"} }
  check  = rest.jq { expr = ".[] | select(.domain_names[0] == \"example.com\")" }
}
```

## Fields

| Field       | Type                  | Required | Description                                      |
| ----------- | --------------------- | :------: | ------------------------------------------------ |
| `method`    | string                |    ✓     | HTTP method (GET, POST, PUT, PATCH, DELETE)      |
| `path`      | string                |    ✓     | Request path (appended to target's `base_url`)   |
| `body`      | `rest.Body?`          |          | Request body — see [below](#body-types)          |
| `headers`   | map\[string, string]? |          | HTTP headers                                     |
| `check`     | `rest.Check?`         |          | Check matcher for idempotency                    |
| `redact`    | list\[string]         |          | jq paths into the response — values are redacted |
| `desc`      | string?               |          | Human-readable description                       |
| `on_change` | list\[Step]           |          | Steps to trigger when this request fires         |

Explicit `headers` take precedence over any headers set automatically by the
body type. For example, `headers = {"Content-Type": "application/json;charset=utf-8"}`
overrides the default `application/json` from `rest.body_json`.

## Body types

### rest.body_json

Serializes a value as JSON. Sets `Content-Type` and `Accept` to
`application/json` (unless overridden via `headers`).

```scampi
body = rest.body_json { data = {"domain_names": ["example.com"]} }
```

### rest.body_string

Sends the content as-is. No automatic headers — set `Content-Type` via the
`headers` field if needed.

```scampi
body = rest.body_string { content = "<xml>raw content</xml>" }
```

## Check matchers

Without a `check`, the request fires on every apply. With a check, scampi
queries the API first and only executes the request if the check is not
satisfied.

### rest.status

Satisfied when the check request returns the expected status code.

```scampi
check = rest.status { code = 200 }
```

### rest.jq

Satisfied when the jq expression produces any non-null, non-false output.

```scampi
check = rest.jq { expr = ".[] | select(.domain == \"example.com\")" }
```

The jq expression runs against the parsed JSON response body. If the check
request returns a non-2xx status, the check is unsatisfied regardless of the
expression.

## How it works

The step produces a single op with check/execute semantics:

1. **Check** — if a check matcher is configured, fires a GET (by default) to
   the same path and evaluates the response. If satisfied, the execute step is
   skipped.
2. **Execute** — fires the configured request (POST, PUT, DELETE, etc). If the
   response status is 4xx or 5xx, the step fails.

Without a check matcher, the step always executes — useful for truly idempotent
endpoints like PUT with a full body.

## Redacting secrets in responses

Some controllers bundle credentials into the same response shape as the config
they manage — UniFi's `setting/mgmt` endpoint returns `x_ssh_password` and a
salted hash alongside fields you actually want to manage. Without help, scampi
prints those values verbatim in `inspect`, drift output, and `-vv` diagnostics.

The `redact` field takes a list of jq-style paths into the response body.
Values at those paths are registered with the renderer's secret redactor as
soon as the response is parsed — every subsequent rendering replaces them with
`***SECRET***`. The redaction applies to every response from this request
regardless of HTTP method (check phase, execute phase, all of it).

```scampi
query = rest.request {
  method = "GET"
  path   = "/proxy/network/api/s/default/get/setting/mgmt"
  check  = rest.jq { expr = ".data[0]" }
  redact = ["data[0].x_ssh_password", "data[0].x_ssh_sha512passwd"]
}
```

Path syntax is jq with an optional leading dot — `x_ssh_password`,
`data.token`, and `items[0].secret` are all valid. Invalid paths surface as
plan-time diagnostics, not runtime surprises.

`redact` works on `rest.request`, on the `query` of `rest.resource`, and on the
`query` of `rest.resource_set`. State-side redaction — marking a key in the
declared `state` map as a secret — is tracked separately and uses a different
mechanism (it's about values scampi puts on the wire, not values scampi reads
back).
