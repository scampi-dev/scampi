---
title: copy
---

Copy files or inline content to the target with owner and permission management.

## Fields

Provide exactly one of:

| Field     | Type   | Description              |
|-----------|--------|--------------------------|
| `src`     | string | Source file path (local)  |
| `content` | string | Inline file content      |

Always required:

| Field   | Type   | Required | Description                                                |
|---------|--------|:--------:|------------------------------------------------------------|
| `dest`  | string |    ✓     | Destination file path (on target)                          |
| `group` | string |    ✓     | Group name or GID                                          |
| `owner` | string |    ✓     | Owner user name or UID                                     |
| `perm`  | string |    ✓     | File permissions (`0644`, `u=rw,g=r,o=r`, or `rw-r--r--`) |
| `desc`  | string |          | Human-readable description                                 |

## How it works

The `copy` step produces three ops that form a dependency chain:

1. **Copy file** — copies the source (or inline content) to the destination
   (compares content bytes)
2. **Set permissions** — ensures file mode matches (depends on #1)
3. **Set ownership** — ensures owner and group match (depends on #1)

The permission and ownership ops only run after the file copy succeeds. If the
file content already matches and permissions and ownership are correct, nothing
happens.

## Examples

### Basic file copy

```python {filename="deploy.star"}
copy(
    src = "./nginx.conf",
    dest = "/etc/nginx/nginx.conf",
    perm = "0644",
    owner = "root",
    group = "root",
)
```

### Inline content

```python {filename="deploy.star"}
copy(
    content = "hal9000 ALL=(ALL) NOPASSWD:ALL\n",
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
    src = "./config.yaml",
    dest = "/opt/myapp/config.yaml",
    perm = "u=rw,g=r,o=",
    owner = "myapp",
    group = "myapp",
)
```

### Restrictive permissions (ls-style)

```python {filename="deploy.star"}
copy(
    src = "./ssl/server.key",
    dest = "/etc/ssl/private/server.key",
    perm = "rw-------",
    owner = "root",
    group = "root",
)
```
