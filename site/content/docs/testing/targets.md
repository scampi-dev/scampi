---
title: Targets
weight: 1
---

Test targets are mock implementations that replace real targets during testing.
They record all operations for assertion verification without touching any
real system.

## In-memory target

`test.target.in_memory()` creates a mock target for POSIX steps (copy, dir,
pkg, service, etc.) with optional pre-populated state:

```starlark
t = test.target.in_memory(
    name = "mock",
    files = {"/etc/hostname": "myhost"},
    packages = ["curl", "git"],
    services = {"sshd": "running", "nginx": "stopped"},
    dirs = ["/var/www", "/home/deploy"],
)
```

| Parameter  | Type   | Description                                 |
| ---------- | ------ | ------------------------------------------- |
| `name`     | string | Target name (required)                      |
| `files`    | dict   | Pre-populated files (path to content)       |
| `packages` | list   | Pre-installed package names                 |
| `services` | dict   | Service states (`"running"` or `"stopped"`) |
| `dirs`     | list   | Pre-existing directory paths                |

All fields except `name` are optional. An empty target represents a fresh
system. Pre-populated state is preserved through the deploy — steps only
modify what they declare.

## REST mock target

`test.target.rest_mock()` creates a mock REST target with pre-configured
routes. Use it to test modules that use `rest.request` or `rest.resource`
steps:

```starlark
t = test.target.rest_mock(
    name = "api",
    routes = {
        "GET /items":      test.response(status=200, body='[{"id":1}]'),
        "POST /items":     test.response(status=201, body='{"id":2}'),
        "DELETE /items/1": test.response(status=204),
    },
)
```

| Parameter | Type   | Description                                    |
| --------- | ------ | ---------------------------------------------- |
| `name`    | string | Target name (required)                         |
| `routes`  | dict   | Route responses (`"METHOD /path"` to response) |

Route keys are `"METHOD /path"` strings. Values are `test.response()`.
Unmatched routes return 404.

### test.response

Defines a mock HTTP response for a route:

```starlark
test.response(status=200, body='{"ok":true}', headers={"X-Custom": "val"})
```

| Parameter | Type   | Description                 |
| --------- | ------ | --------------------------- |
| `status`  | int    | HTTP status code (required) |
| `body`    | string | Response body               |
| `headers` | dict   | Response headers            |
