---
title: Assertions
weight: 2
---

`test.assert.that(t)` returns an assertion builder for a test target.
Chain a resource selector with a check method:

```starlark
a = test.assert.that(t)

a.file("/etc/app.conf").exists()
a.service("nginx").is_running()
a.request("POST", "/items").was_called()
```

## File

| Method              | Checks                       |
| ------------------- | ---------------------------- |
| `.file(path)`       | Select a file for assertions |
| `.has_content(str)` | Exact content match          |
| `.contains(str)`    | Substring match              |
| `.exists()`         | File must exist              |
| `.absent()`         | File must not exist          |
| `.has_mode("0644")` | Permission check             |

## Directory

| Method              | Checks                   |
| ------------------- | ------------------------ |
| `.dir(path)`        | Select a directory       |
| `.exists()`         | Directory must exist     |
| `.absent()`         | Directory must not exist |
| `.has_mode("0755")` | Permission check         |

## Service

| Method           | Checks                         |
| ---------------- | ------------------------------ |
| `.service(name)` | Select a service               |
| `.is_running()`  | Service is active              |
| `.is_stopped()`  | Service is inactive            |
| `.is_enabled()`  | Service starts at boot         |
| `.is_disabled()` | Service does not start at boot |

## Package

| Method            | Checks                   |
| ----------------- | ------------------------ |
| `.package(name)`  | Select a package         |
| `.is_installed()` | Package is present       |
| `.is_absent()`    | Package is not installed |

## Symlink

| Method               | Checks                 |
| -------------------- | ---------------------- |
| `.symlink(path)`     | Select a symlink       |
| `.points_to(target)` | Symlink target matches |
| `.absent()`          | Symlink must not exist |

## Container

| Method              | Checks                         |
| ------------------- | ------------------------------ |
| `.container(name)`  | Select a container             |
| `.is_running()`     | Container is running           |
| `.has_image(image)` | Container uses the given image |

## Command

| Method                    | Checks                             |
| ------------------------- | ---------------------------------- |
| `.command_ran(substring)` | A command containing substring ran |

## Request

Request assertions work with `test.target.rest_mock()` targets. They verify
which HTTP requests were made during execution.

| Method                        | Checks                                           |
| ----------------------------- | ------------------------------------------------ |
| `.request(method, path)`      | Select requests matching method and path         |
| `.was_called()`               | At least one matching request was made           |
| `.was_called_times(n)`        | Exactly n matching requests were made            |
| `.was_not_called()`           | No matching requests were made                   |
| `.body_contains(str)`         | Last matching request body contains substring    |
| `.header_equals(name, value)` | Last matching request has the given header value |

### Example

```starlark
t = test.target.rest_mock(
    name = "api",
    routes = {
        "POST /items": test.response(status=201, body='{"id":1}'),
    },
)

deploy(
    name = "test",
    targets = ["api"],
    steps = [
        rest.request(
            method = "POST",
            path = "/items",
            body = rest.body.json({"name": "foo"}),
            check = rest.status(code=201),
        ),
    ],
)

a = test.assert.that(t)
a.request("POST", "/items").was_called()
a.request("POST", "/items").body_contains('"name"')
a.request("GET", "/items").was_not_called()
```
