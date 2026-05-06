---
title: symlink
---

Create and manage symbolic links on the target.

## Fields

| Field       | Type          | Required | Description                                               |
| ----------- | ------------- | :------: | --------------------------------------------------------- |
| `target`    | string        |    ✓     | Path the symlink points to (`@std.path(absolute=true)`)   |
| `link`      | string        |    ✓     | Path where the symlink lives (`@std.path(absolute=true)`) |
| `desc`      | string?       |          | Human-readable description                                |
| `on_change` | list\[Step]   |          | Steps to trigger when the link is created or updated      |
| `promises`  | list\[string] |          | Cross-deploy resources this step produces                 |
| `inputs`    | list\[string] |          | Cross-deploy resources this step consumes                 |

Think of it like `ln -sf TARGET LINK` — `target` is what you're pointing at,
`link` is where the symlink lives.

## How it works

`symlink` reconciles `link` to be a symlink pointing at `target`. If
`link` is missing, has the wrong target, or is a regular file (or
empty directory), scampi removes whatever's there and creates the
symlink.

Removing a non-empty directory is refused — the underlying filesystem
returns `ENOTEMPTY` and the step fails with a clean error rather than
silently nuking a directory tree.

## Examples

### Config symlink

```scampi
posix.symlink {
  target = "/opt/app/current/config.yaml"
  link   = "/etc/app/config.yaml"
}
```

### Version switching

```scampi
posix.symlink {
  desc   = "point to active release"
  target = "/opt/releases/v1.4.2"
  link   = "/opt/app/current"
}
```

### Reload service when link changes

```scampi
let reload_app = posix.service {
  name  = "myapp"
  state = posix.ServiceState.reloaded
}

posix.symlink {
  target    = "/opt/releases/v1.4.2"
  link      = "/opt/app/current"
  on_change = [reload_app]
}
```
