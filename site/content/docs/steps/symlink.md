---
title: symlink
---

Create and manage symbolic links on the target.

## Fields

| Field    | Type   | Required | Description |
|----------|--------|:--------:|-------------|
| `link`   | string | ✓ | Path where the symlink is created |
| `target` | string | ✓ | Path the symlink points to (the file being linked to) |
| `desc`   | string |   | Human-readable description |

Think of it like `ln -s TARGET LINK` — `target` is what you're pointing at,
`link` is where the symlink lives.

The `link` path must be absolute. The `target` path can be relative (it's
resolved relative to the link's parent directory).

## How it works

The `symlink` step checks if the link already exists and points to the correct
target. If the link is missing or points somewhere else, it creates (or
recreates) it.

## Examples

### Config symlink

```python
symlink(
    target = "/opt/app/current/config.yaml",
    link = "/etc/app/config.yaml",
)
```

### Version switching

```python
symlink(
    desc = "point to active release",
    target = "/opt/releases/v1.4.2",
    link = "/opt/app/current",
)
```

### Relative target

```python
symlink(
    target = "../shared/logging.conf",
    link = "/opt/app/config/logging.conf",
)
```
