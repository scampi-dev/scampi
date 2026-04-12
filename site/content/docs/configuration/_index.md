---
title: Configuration
weight: 3
---

scampi configs are `.scampi` files written in the scampi language. See the
[Language guide]({{< relref "../language" >}}) for syntax details.

## Targets

A target defines where steps execute. Import a target module, bind it with
`let`, and pass it to `std.deploy(...)`:

```scampi
import "std/ssh"

let web = ssh.target { name = "web", host = "app.example.com", user = "deploy" }
```

See the [Target Reference]({{< relref "../targets" >}}) for all available
target types and their fields.

## Deploy blocks

`std.deploy(...)` is a function that takes a name and target list, followed by
a trailing block containing the steps to execute:

```scampi
std.deploy(name = "webserver", targets = [web]) {
  posix.pkg {
    packages = ["nginx"]
    source   = posix.pkg_system {}
  }

  posix.service { name = "nginx", state = posix.ServiceState.running, enabled = true }
}
```

| Field     | Type           | Required | Description                   |
| --------- | -------------- | :------: | ----------------------------- |
| `name`    | string         |    ✓     | Unique identifier             |
| `targets` | list\[Target]  |    ✓     | Targets to execute against    |

Steps inside the trailing block execute in order. Each step becomes an action
in the plan.

## Source resolvers

Steps that work with content — like `copy`, `template`, and `unarchive` —
accept a `src` field. The value is a **source resolver** that tells scampi
where the content lives. All source resolvers live in the `posix` module.

### posix.source_local

Reads a file from the local machine, relative to the scampi file:

```scampi
posix.copy {
  src   = posix.source_local { path = "./files/nginx.conf" }
  dest  = "/etc/nginx/nginx.conf"
  perm  = "0644", owner = "root", group = "root"
}
```

### posix.source_inline

Uses a string literal as content. Useful for small configs where a separate file
is overkill:

```scampi
posix.copy {
  src   = posix.source_inline { content = "nameserver 1.1.1.1\nnameserver 1.0.0.1\n" }
  dest  = "/etc/resolv.conf"
  perm  = "0644", owner = "root", group = "root"
}
```

### posix.source_remote

Fetches a URL via HTTP/HTTPS. Downloaded content is cached locally so repeated
runs don't re-download unless the remote has changed (uses conditional requests
via ETag/Last-Modified).

```scampi
posix.copy {
  src   = posix.source_remote { url = "https://example.com/config.yaml" }
  dest  = "/etc/app/config.yaml"
  perm  = "0644", owner = "root", group = "root"
}
```

| Field      | Type    | Required | Description                          |
| ---------- | ------- | :------: | ------------------------------------ |
| `url`      | string  |    ✓     | HTTP or HTTPS URL                    |
| `checksum` | string? |          | Expected digest in `algo:hex` format |

Supported checksum algorithms: `sha256`, `sha384`, `sha512`, `sha1`, `md5`.

When a checksum is provided, scampi verifies the downloaded content matches
before using it. If it doesn't match, the step fails with a clear error — the
file on the target is never modified.

```scampi
posix.unarchive {
  src = posix.source_remote {
    url      = "https://github.com/caddyserver/caddy/releases/download/v2.9.1/caddy_2.9.1_linux_amd64.tar.gz"
    checksum = "sha256:a8f23e58ba52c3547e0c0e64be46419e8a8aa52b1bae3eb23c485c3b2a512c55"
  }
  dest  = "/opt/caddy"
  owner = "caddy", group = "caddy"
}
```

All source resolvers compose with all steps that accept `src`. See
[Concepts]({{< relref "../concepts" >}}) for why this matters.

## Environment variables

Use `std.env()` to read environment variables in your config:

```scampi
posix.template {
  src  = posix.source_inline { content = "DB_HOST={{ .values.db_host }}" }
  dest = "/opt/app/.env"
  perm = "0600", owner = "app", group = "app"
  data = {"values": {"db_host": std.env("DB_HOST")}}
}
```

`std.env("DB_HOST")` reads the environment variable at evaluation time and
passes it into the template data.

## Secrets

Secrets are managed through a two-step setup: configure a backend with
`std.secrets`, then reference individual values with `std.secret(...)`.

### Configure a backend

```scampi
std.secrets { backend = std.SecretsBackend.age, path = "secrets.age.json" }
```

Currently supported backends:

| Backend                    | Description                                                             |
| -------------------------- | ----------------------------------------------------------------------- |
| `SecretsBackend.file`      | Plain JSON key-value file (unencrypted)                                 |
| `SecretsBackend.age`       | Encrypted JSON file using [age encryption](https://age-encryption.org/) |

The `std.secrets` call can only appear once per config.

### Reference a secret

```scampi
posix.template {
  src  = posix.source_inline { content = "DATABASE_URL=postgres://app:{{ .values.db_password }}@db:5432/app" }
  dest = "/opt/app/.env"
  perm = "0600", owner = "app", group = "app"
  data = {"values": {"db_password": std.secret("db_password")}}
}
```

Secret values are resolved at runtime and **never appear in any output** — not
in plans, not in check results, not in logs. scampi never writes them to disk or
stores them anywhere; they exist in memory only. The only way a secret
materializes on the target is if your config explicitly puts it there (e.g. via
a template that writes it into a file).

For CLI flags and subcommands, see the [CLI reference](../cli).
