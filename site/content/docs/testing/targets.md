---
title: Targets
weight: 1
---

Test targets are mock implementations that replace real targets during testing.
They record all operations for verification without touching any real system.

## In-memory target

`test.target_in_memory(...)` creates a mock target for POSIX steps (copy, dir,
pkg, service, etc.) with optional pre-populated state and expected end state:

```scampi
import "std/test"
import "std/test/matchers"
import "std/posix"

let mock = test.target_in_memory(
  name    = "mock",
  initial = test.InitialState {
    files    = {"/etc/hostname": posix.source_inline { content = "myhost" }}
    packages = ["curl", "git"]
    services = {"sshd": posix.ServiceState.running, "nginx": posix.ServiceState.stopped}
    dirs     = ["/var/www", "/home/deploy"]
  },
  expect = test.ExpectedState {
    files    = {"/etc/nginx/nginx.conf": matchers.is_present()}
    services = {"nginx": matchers.has_svc_status(posix.ServiceState.running)}
  },
)
```

### InitialState

Seed the mock target with existing state before the deploy runs.

| Field      | Type                              | Description                                  |
| ---------- | --------------------------------- | -------------------------------------------- |
| `files`    | map\[string, posix.Source]?       | Pre-populated files (path to source content) |
| `packages` | list\[string]?                    | Pre-installed package names                  |
| `services` | map\[string, posix.ServiceState]? | Service states                               |
| `dirs`     | list\[string]?                    | Pre-existing directory paths                 |
| `symlinks` | map\[string, string]?             | Pre-existing symlinks (link to target)       |

All fields are optional. An empty initial state (or omitting it entirely)
represents a fresh system.

### ExpectedState

Declare what should be true after the deploy. Every entry is a
`matchers.Matcher` — see [Matchers]({{< relref "assertions" >}}) for the
full list.

| Field      | Type                              | Description                      |
| ---------- | --------------------------------- | -------------------------------- |
| `files`    | map\[string, matchers.Matcher]?   | Expected file state              |
| `packages` | map\[string, matchers.Matcher]?   | Expected package state           |
| `services` | map\[string, matchers.Matcher]?   | Expected service state           |
| `dirs`     | map\[string, matchers.Matcher]?   | Expected directory state         |
| `symlinks` | map\[string, matchers.Matcher]?   | Expected symlink state           |

Only listed entries are verified — unlisted slots are unconstrained. Use
`matchers.is_absent()` to explicitly assert something should NOT exist.

`expect` is optional — omit it for smoke tests that only verify the deploy
applies cleanly.

## REST mock target

`test.target_rest_mock(...)` creates a mock REST target with pre-configured
routes and optional request verification:

```scampi
import "std/test"

let api = test.target_rest_mock(
  name     = "api",
  base_url = "https://api.example.com",
  routes   = {
    "GET /items":      test.response(status = 200, body = "[{\"id\":1}]"),
    "POST /items":     test.response(status = 201, body = "{\"id\":2}"),
    "DELETE /items/1": test.response(status = 204),
  },
  expect_requests = [
    test.request(method = "POST", path = "/items"),
  ],
)
```

| Parameter         | Type                           | Description                                    |
| ----------------- | ------------------------------ | ---------------------------------------------- |
| `name`            | string                         | Target name (required)                         |
| `base_url`        | string                         | Base URL for the mock                          |
| `routes`          | map\[string, test.Response]    | Route responses (`"METHOD /path"` to response) |
| `expect_requests` | list\[test.RequestMatcher]?    | Request matchers for verification              |

Route keys are `"METHOD /path"` strings. Unmatched routes return 404.

### test.response

Defines a mock HTTP response for a route:

```scampi
test.response(status = 200, body = "{\"ok\":true}", headers = {"X-Custom": "val"})
```

| Parameter | Type                   | Required | Description                 |
| --------- | ---------------------- | :------: | --------------------------- |
| `status`  | int                    |    ✓     | HTTP status code            |
| `body`    | string?                |          | Response body               |
| `headers` | map\[string, string]?  |          | Response headers            |

### test.request

Defines a request matcher for `expect_requests`:

```scampi
test.request(method = "POST", path = "/items", count = 1)
```

| Parameter       | Type              | Description                         |
| --------------- | ----------------- | ----------------------------------- |
| `method`        | string            | HTTP method to match                |
| `path`          | string            | Request path to match               |
| `body`          | matchers.Matcher? | Body content matcher                |
| `count`         | int?              | Expect exactly N matching requests  |
| `count_at_least`| int?              | Expect at least N (default: 1)      |
