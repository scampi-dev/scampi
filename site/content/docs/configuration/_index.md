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

| Field  | Required | Description |
|--------|:--------:|-------------|
| `host` | ✓ | Hostname or IP address |
| `name` | ✓ | Identifier referenced by deploy blocks |
| `user` | ✓ | SSH user |

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

| Field     | Required | Description |
|-----------|:--------:|-------------|
| `name`    | ✓ | Unique identifier for this deploy block |
| `steps`   | ✓ | Ordered list of steps to execute |
| `targets` |   | List of target names (omit for local execution) |

Steps within a deploy block execute in order. Each step becomes an action in the
plan.

## Environment variables

Use `env()` to read environment variables in your config:

```python
deploy(
    name = "app",
    steps = [
        template(
            content = "DB_HOST={{ .values.db_host }}",
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

| Backend | Description |
|---------|-------------|
| `file`  | Plain JSON key-value file (unencrypted) |
| `age`   | Encrypted JSON file using [age encryption](https://age-encryption.org/) |

The `secrets()` call can only appear once per config.

### Reference a secret

```python
template(
    content = "DATABASE_URL=postgres://app:{{ .values.db_password }}@db:5432/app",
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
