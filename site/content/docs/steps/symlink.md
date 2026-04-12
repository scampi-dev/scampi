---
title: symlink
---

Create and manage symbolic links on the target.

## Fields

| Field       | Type        | Required | Description                                               |
| ----------- | ----------- | :------: | --------------------------------------------------------- |
| `target`    | string      |    ✓     | Path the symlink points to (`@std.path(absolute=true)`)   |
| `link`      | string      |    ✓     | Path where the symlink lives (`@std.path(absolute=true)`) |
| `desc`      | string?     |          | Human-readable description                                |
| `on_change` | list\[Step] |          | Steps to trigger when the link is created or updated      |

Think of it like `ln -s TARGET LINK` — `target` is what you're pointing at,
`link` is where the symlink lives.

## How it works

The `symlink` step checks if the link already exists and points to the correct
target. If the link is missing or points somewhere else, it creates (or
recreates) it.

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
