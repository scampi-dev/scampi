---
title: copy
---

Copy files or inline content to the target with owner and permission management.

## Fields

| Field      | Type   | Required | Description                                                        |
|------------|--------|:--------:|--------------------------------------------------------------------|
| `src`      | source |    ✓     | Source resolver: `local("./path")` or `inline("content")`          |
| `dest`     | string |    ✓     | Destination file path (on target)                                  |
| `group`    | string |    ✓     | Group name or GID                                                  |
| `owner`    | string |    ✓     | Owner user name or UID                                             |
| `perm`     | string |    ✓     | File permissions (`0644`, `u=rw,g=r,o=r`, or `rw-r--r--`)          |
| `desc`     | string |          | Human-readable description                                         |
| `verify`   | string |          | Command to validate content before writing (`%s` = temp file path) |

## Source resolvers

The `src` field accepts a source resolver:

- **`local("./path")`** — reads a file from the local machine relative to the
  Starlark file
- **`inline("content")`** — uses the given string as file content (written to a
  cache file at eval time)

## How it works

The `copy` step produces three ops that form a dependency chain:

1. **Copy file** — copies the source to the destination (compares content bytes)
2. **Set permissions** — ensures file mode matches (depends on #1)
3. **Set ownership** — ensures owner and group match (depends on #1)

The permission and ownership ops only run after the file copy succeeds. If the
file content already matches and permissions and ownership are correct, nothing
happens.

### Verify

When `verify` is set, the content is written to a temporary file first. The
verify command runs with `%s` replaced by the temp file path. If the command
exits 0, the content is written to the destination. If it exits non-zero, the
destination is left untouched and the step fails. The temp file is always
cleaned up.

Verify only runs when the content actually needs to change — idempotent runs
skip it entirely.

## Examples

### Basic file copy

```python {filename="deploy.star"}
copy(
    src = local("./nginx.conf"),
    dest = "/etc/nginx/nginx.conf",
    perm = "0644",
    owner = "root",
    group = "root",
)
```

### Inline content

```python {filename="deploy.star"}
copy(
    src = inline("hal9000 ALL=(ALL) NOPASSWD:ALL\n"),
    dest = "/etc/sudoers.d/hal9000",
    perm = "0440",
    owner = "root",
    group = "root",
    desc = "passwordless sudo for automation user",
)
```

### Application config (POSIX notation)

```python {filename="deploy.star"}
copy(
    desc = "deploy app config",
    src = local("./config.yaml"),
    dest = "/opt/myapp/config.yaml",
    perm = "u=rw,g=r,o=",
    owner = "myapp",
    group = "myapp",
)
```

### Validated sudoers file

```python {filename="deploy.star"}
copy(
    src = inline("hal9000 ALL=(ALL) NOPASSWD:ALL\n"),
    dest = "/etc/sudoers.d/hal9000",
    perm = "0440",
    owner = "root",
    group = "root",
    verify = "visudo -cf %s",
)
```

### Validated nginx config

```python {filename="deploy.star"}
copy(
    src = local("./nginx.conf"),
    dest = "/etc/nginx/nginx.conf",
    perm = "0644",
    owner = "root",
    group = "root",
    verify = "nginx -t -c %s",
)
```

### Restrictive permissions (ls-style)

```python {filename="deploy.star"}
copy(
    src = local("./ssl/server.key"),
    dest = "/etc/ssl/private/server.key",
    perm = "rw-------",
    owner = "root",
    group = "root",
)
```
