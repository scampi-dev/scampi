---
title: Testing
weight: 3
---

`scampi test` runs Starlark test files against in-memory targets. Test files
are regular scampi configs — same `deploy()`, same steps, same `load()` — with
test builtins for setting up mock state and asserting outcomes.

## Writing tests

A test file is any `*_test.scampi` file. It uses `test.target.in_memory()` instead
of a real target and adds assertions after the deploy block:

```starlark {filename="webserver_test.scampi"}
load("webserver.scampi", "static_site")

t = test.target.in_memory(
    name = "mock",
    packages = ["nginx"],
    services = {"nginx": "stopped"},
)

deploy(
    name = "webserver",
    targets = ["mock"],
    steps = [
        static_site(domain = "example.com", root = "/var/www/mysite"),
    ],
)

a = test.assert.that(t)

a.file("/etc/nginx/sites-enabled/example.com.conf").contains("server_name example.com")
a.service("nginx").is_running()
a.dir("/var/www/mysite").exists()
```

## Running tests

```text
scampi test                          # *_test.scampi in current directory
scampi test ./...                    # recursive from current directory
scampi test path/to/test.scampi        # specific file
scampi test path/to/dir              # all *_test.scampi in that directory
scampi test path/to/dir/...          # recursive from that directory
```

All assertions run even if some fail — you see the full picture in one run.
Exit code 0 if all pass, 1 if any fail.

## In-memory target

`test.target.in_memory()` creates a mock target with optional pre-populated state:

```starlark
t = test.target.in_memory(
    name = "mock",
    files = {"/etc/hostname": "myhost"},
    packages = ["curl", "git"],
    services = {"sshd": "running", "nginx": "stopped"},
    dirs = ["/var/www", "/home/deploy"],
)
```

All fields are optional. An empty target represents a fresh system.

Pre-populated state is preserved through the deploy — steps only modify
what they declare.

## Assertions

`test.assert.that(t)` returns an assertion builder. Chain resource type and
check method:

```starlark
a = test.assert.that(t)
```

### File assertions

| Method              | Checks                       |
| ------------------- | ---------------------------- |
| `.file(path)`       | Select a file for assertions |
| `.has_content(str)` | Exact content match          |
| `.contains(str)`    | Substring match              |
| `.exists()`         | File must exist              |
| `.absent()`         | File must not exist          |
| `.has_mode("0644")` | Permission check             |

### Directory assertions

| Method              | Checks                   |
| ------------------- | ------------------------ |
| `.dir(path)`        | Select a directory       |
| `.exists()`         | Directory must exist     |
| `.absent()`         | Directory must not exist |
| `.has_mode("0755")` | Permission check         |

### Service assertions

| Method           | Checks                         |
| ---------------- | ------------------------------ |
| `.service(name)` | Select a service               |
| `.is_running()`  | Service is active              |
| `.is_stopped()`  | Service is inactive            |
| `.is_enabled()`  | Service starts at boot         |
| `.is_disabled()` | Service does not start at boot |

### Package assertions

| Method            | Checks                   |
| ----------------- | ------------------------ |
| `.package(name)`  | Select a package         |
| `.is_installed()` | Package is present       |
| `.is_absent()`    | Package is not installed |

### Symlink assertions

| Method               | Checks                 |
| -------------------- | ---------------------- |
| `.symlink(path)`     | Select a symlink       |
| `.points_to(target)` | Symlink target matches |
| `.absent()`          | Symlink must not exist |

### Container assertions

| Method              | Checks                         |
| ------------------- | ------------------------------ |
| `.container(name)`  | Select a container             |
| `.is_running()`     | Container is running           |
| `.has_image(image)` | Container uses the given image |

### Command assertions

| Method                    | Checks                             |
| ------------------------- | ---------------------------------- |
| `.command_ran(substring)` | A command containing substring ran |

## Composable test helpers

Since Starlark is a real language, you can write reusable assertion helpers:

```starlark
def assert_nginx(t, domain):
    a = test.assert.that(t)
    a.service("nginx").is_running()
    a.file("/etc/nginx/sites-enabled/" + domain + ".conf").contains(
        "server_name " + domain,
    )
```

## Multiple deploy blocks

Test files can contain multiple deploy blocks against the same target.
Assertions run after all deploy blocks have been applied:

```starlark
t = test.target.in_memory(name = "mock")

deploy(name = "base", targets = ["mock"], steps = [...])
deploy(name = "app", targets = ["mock"], steps = [...])

a = test.assert.that(t)
# assertions check the state after both deploys ran
```
