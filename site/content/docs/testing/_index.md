---
title: Testing
weight: 6
---

`scampi test` runs Starlark test files against mock targets. Test files are
regular scampi configs — same `deploy()`, same steps, same `load()` — with
test builtins for setting up mock state and asserting outcomes.

## Writing tests

A test file is any `*_test.scampi` file. It sets up a mock target, deploys
steps against it, then asserts the outcome:

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
scampi test                        # *_test.scampi in current directory
scampi test ./...                  # recursive from current directory
scampi test path/to/test.scampi   # specific file
scampi test path/to/dir           # all *_test.scampi in that directory
scampi test path/to/dir/...       # recursive from that directory
```

All assertions run even if some fail — you see the full picture in one run.
Exit code 0 if all pass, 1 if any fail.

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

## Sections

{{< cards >}}
  {{< card link="targets" title="Targets" subtitle="In-memory and REST mock targets for testing" >}}
  {{< card link="assertions" title="Assertions" subtitle="File, service, package, container, and request assertions" >}}
{{< /cards >}}
