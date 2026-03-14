---
title: copy
---

Copy files from the local machine to the target with owner and permission
management.

## Fields

| Field   | Type   | Required | Description |
|---------|--------|:--------:|-------------|
| `dest`  | string | ✓ | Destination file path (on target) |
| `group` | string | ✓ | Group name or GID |
| `owner` | string | ✓ | Owner user name or UID |
| `perm`  | string | ✓ | File permissions (`0644`, `u=rw,g=r,o=r`, or `rw-r--r--`) |
| `src`   | string | ✓ | Source file path (local) |
| `desc`  | string |   | Human-readable description |

## How it works

The `copy` step produces three ops that form a dependency chain:

1. **Copy file** — copies the source to the destination (compares content hashes)
2. **Set permissions** — ensures file mode matches (depends on #1)
3. **Set ownership** — ensures owner and group match (depends on #1)

The permission and ownership ops only run after the file copy succeeds. If the
file content already matches and permissions and ownership are correct, nothing
happens.

## Examples

### Basic file copy

```python
copy(
    src = "./nginx.conf",
    dest = "/etc/nginx/nginx.conf",
    perm = "0644",
    owner = "root",
    group = "root",
)
```

### Application config (POSIX notation)

```python
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

```python
copy(
    src = "./ssl/server.key",
    dest = "/etc/ssl/private/server.key",
    perm = "rw-------",
    owner = "root",
    group = "root",
)
```
