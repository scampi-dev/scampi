---
title: dir
---

Ensure a directory exists on the target, optionally with specific permissions and
ownership. Creates parent directories as needed.

## Fields

| Field   | Type   | Required | Description |
|---------|--------|:--------:|-------------|
| `path`  | string | ✓ | Absolute path to ensure exists (creates parents) |
| `desc`  | string |   | Human-readable description |
| `group` | string |   | Group name or GID |
| `owner` | string |   | Owner user name or UID |
| `perm`  | string |   | File permissions (`0755`, `u=rwx,g=rx,o=rx`, or `rwxr-xr-x`) |

If `owner` is set, `group` must also be set (and vice versa).

## How it works

The `dir` step produces up to three ops:

1. **Ensure directory** — creates the directory and parents if missing
2. **Set permissions** — ensures mode matches (depends on #1, only if `perm` set)
3. **Set ownership** — ensures owner/group match (depends on #1, only if `owner`/`group` set)

## Examples

### Simple directory

```python
dir(path="/var/www/mysite")
```

### With permissions

```python
dir(
    path = "/opt/app/data",
    perm = "0755",
)
```

### With ownership

```python
dir(
    path = "/opt/app/data",
    perm = "0750",
    owner = "app",
    group = "app",
)
```
