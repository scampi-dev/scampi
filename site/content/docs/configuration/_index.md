---
title: Configuration
weight: 3
---

Scampi configs are Starlark files (`.star`). Starlark is a deterministic,
Python-like language — if you know Python, you already know Starlark.

## Targets

A target defines where steps execute. Currently supported:

### Local

```python {filename="deploy.star"}
target.local(name="my-machine")
```

The local target runs steps on the machine where scampi is invoked. There can
only be one local target — you only have one local machine.

### SSH

```python {filename="deploy.star"}
target.ssh(
    name = "web",
    host = "app.example.com",
    user = "deploy",
)
```

SSH targets connect to a remote host. Authentication uses your SSH agent or key
files.

| Field  | Required | Description                            |
|--------|:--------:|----------------------------------------|
| `host` |    ✓     | Hostname or IP address                 |
| `name` |    ✓     | Identifier referenced by deploy blocks |
| `user` |    ✓     | SSH user                               |

## Deploy blocks

A `deploy()` block binds a list of steps to one or more targets:

```python
deploy(
    name = "webserver",
    targets = ["web"],
    steps = [
        pkg(packages=["nginx"], state="present"),
        service(name="nginx", state="running", enabled=True),
    ],
)
```

| Field     | Required | Description                                     |
|-----------|:--------:|-------------------------------------------------|
| `name`    |    ✓     | Unique identifier for this deploy block         |
| `steps`   |    ✓     | Ordered list of steps to execute                |
| `targets` |          | List of target names (omit for local execution) |

Steps within a deploy block execute in order. Each step becomes an action in the
plan.

## Source resolvers

Steps that work with content — like `copy`, `template`, and `unarchive` —
accept a `src` field. The value is a **source resolver** — a function that tells
scampi where the content lives.

### local

Reads a file from the local machine, relative to the Starlark file:

```python
copy(
    src = local("./files/nginx.conf"),
    dest = "/etc/nginx/nginx.conf",
    perm = "0644", owner = "root", group = "root",
)
```

### inline

Uses a string literal as content. Useful for small configs where a separate file
is overkill:

```python
copy(
    src = inline("nameserver 1.1.1.1\nnameserver 1.0.0.1\n"),
    dest = "/etc/resolv.conf",
    perm = "0644", owner = "root", group = "root",
)
```

### remote

Fetches a URL via HTTP/HTTPS. Downloaded content is cached locally so repeated
runs don't re-download unless the remote has changed (uses conditional requests
via ETag/Last-Modified).

```python
copy(
    src = remote(url="https://example.com/config.yaml"),
    dest = "/etc/app/config.yaml",
    perm = "0644", owner = "root", group = "root",
)
```

| Parameter  | Required | Description                               |
|------------|:--------:|-------------------------------------------|
| `url`      |    ✓     | HTTP or HTTPS URL                         |
| `checksum` |          | Expected digest in `algo:hex` format      |

Supported checksum algorithms: `sha256`, `sha384`, `sha512`, `sha1`, `md5`.

When a checksum is provided, scampi verifies the downloaded content matches
before using it. If it doesn't match, the step fails with a clear error — the
file on the target is never modified.

```python
unarchive(
    src = remote(
        url = "https://github.com/caddyserver/caddy/releases/download/v2.9.1/caddy_2.9.1_linux_amd64.tar.gz",
        checksum = "sha256:a8f23e58ba52c3547e0c0e64be46419e8a8aa52b1bae3eb23c485c3b2a512c55",
    ),
    dest = "/opt/caddy",
    owner = "caddy", group = "caddy",
)
```

All source resolvers compose with all steps that accept `src`. See
[Concepts]({{< relref "../concepts" >}}) for why this matters.

## Environment variables

Use `env()` to read environment variables in your config:

```python
deploy(
    name = "app",
    steps = [
        template(
            src = inline("DB_HOST={{ .values.db_host }}"),
            dest = "/opt/app/.env",
            perm = "0600", owner = "app", group = "app",
            data = {"values": {"db_host": env("DB_HOST")}},
        ),
    ],
)
```

The `env("DB_HOST")` reads the environment variable at runtime and passes it
into the template as `.values.db_host`.

## Secrets

Secrets are managed through a two-step setup: configure a backend with
`secrets()`, then reference individual values with `secret()`.

### Configure a backend

```python {filename="deploy.star"}
secrets(backend="age", path="secrets.age.json")
```

Currently supported backends:

| Backend | Description                                                             |
|---------|-------------------------------------------------------------------------|
| `file`  | Plain JSON key-value file (unencrypted)                                 |
| `age`   | Encrypted JSON file using [age encryption](https://age-encryption.org/) |

The `secrets()` call can only appear once per config.

### Reference a secret

```python
template(
    src = inline("DATABASE_URL=postgres://app:{{ .values.db_password }}@db:5432/app"),
    dest = "/opt/app/.env",
    perm = "0600", owner = "app", group = "app",
    data = {"values": {"db_password": secret("db_password")}},
)
```

Secret values are resolved at runtime and **never appear in any output** — not
in plans, not in check results, not in logs. Scampi never writes them to disk or
stores them anywhere; they exist in memory only. The only way a secret
materializes on the target is if your config explicitly puts it there (e.g. via
a template that writes it into a file).

## Multi-file configs

Use `load()` to split configs across files:

```python {filename="deploy.star"}
load("targets.star", "web", "db")

deploy(
    name = "app",
    targets = ["web"],
    steps = [ ... ],
)
```

This works exactly like Starlark's standard `load()` statement
([spec](https://github.com/google/starlark-go/blob/master/doc/spec.md#load-statements))
— it imports named bindings from another file.

For CLI flags and subcommands, see the [CLI reference](../cli).
