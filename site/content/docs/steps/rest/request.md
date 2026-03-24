---
title: request
---

Make HTTP requests against a REST target with optional idempotency checks.
See the [rest module overview](../) for check matchers and general concepts.

```python
rest.request(
    desc = "create proxy host",
    method = "POST",
    path = "/nginx/proxy-hosts",
    body = rest.body.json({"domain_names": ["example.com"], "forward_host": "10.10.2.5"}),
    check = rest.jq(expr='.[] | select(.domain_names[0] == "example.com")'),
)
```

## Fields

| Field     | Required | Description                                    |
| --------- | :------: | ---------------------------------------------- |
| `method`  |    ✓     | HTTP method (GET, POST, PUT, PATCH, DELETE)    |
| `path`    |    ✓     | Request path (appended to target's `base_url`) |
| `body`    |          | Request body (see below)                       |
| `headers` |          | HTTP headers (dict of string to string)        |
| `check`   |          | Check matcher for idempotency (see below)      |
| `desc`    |          | Human-readable description                     |

Explicit `headers` take precedence over any headers set automatically by the
body type. For example, `headers={"Content-Type": "application/json;charset=utf-8"}`
overrides the default `application/json` from `rest.body.json`.

## Body types

### rest.body.json

Serializes a dict as JSON. Sets `Content-Type` and `Accept` to
`application/json` (unless overridden via `headers`).

```python
body = rest.body.json({"domain_names": ["example.com"]})
```

### rest.body.string

Sends the content as-is. No automatic headers — set `Content-Type` via the
`headers` field if needed.

```python
body = rest.body.string("<xml>raw content</xml>")
```

## Check matchers

Without a `check`, the request fires on every apply. With a check, scampi
queries the API first and only executes the request if the check is not
satisfied.

### rest.status

Satisfied when the check request returns the expected status code.

```python
check = rest.status(code=200)
```

| Field    | Required | Default         | Description                       |
| -------- | :------: | --------------- | --------------------------------- |
| `code`   |    ✓     |                 | Expected HTTP status code         |
| `path`   |          | same as request | Override the check request path   |
| `method` |          | `"GET"`         | Override the check request method |

### rest.jq

Satisfied when the jq expression produces any non-null, non-false output.

```python
check = rest.jq(expr='.[] | select(.domain == "example.com")')
```

| Field    | Required | Default         | Description                       |
| -------- | :------: | --------------- | --------------------------------- |
| `expr`   |    ✓     |                 | jq expression to evaluate         |
| `path`   |          | same as request | Override the check request path   |
| `method` |          | `"GET"`         | Override the check request method |

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
