---
title: service
---

Manage service state: running, stopped, restarted, or reloaded.
Works with systemd, OpenRC, and launchctl.

## Fields

| Field       | Type             | Required | Default                  | Description                              |
| ----------- | ---------------- | :------: | ------------------------ | ---------------------------------------- |
| `name`      | string           |    ✓     |                          | Service name (`@std.nonempty`)           |
| `state`     | `ServiceState`   |          | `ServiceState.running`   | Desired state — see [below](#states)     |
| `enabled`   | bool             |          | `true`                   | Whether the service should start at boot |
| `desc`      | string?          |          |                          | Human-readable description               |
| `on_change` | list\[Step]      |          |                          | Steps to trigger when this service changes |

## States

`posix.ServiceState` is an enum:

| Value                          | Behavior                                                                                                           |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| `ServiceState.running`         | Start the service if not active. Idempotent.                                                                       |
| `ServiceState.stopped`         | Stop the service if active. Idempotent.                                                                            |
| `ServiceState.restarted`       | Restart the service unconditionally. Always fires.                                                                 |
| `ServiceState.reloaded`        | Reload the service unconditionally. Falls back to restart if the init system doesn't support reload. Always fires. |

## How it works

For `running` and `stopped`, the step produces two independent ops that run in
parallel:

1. **Ensure active state** — start or stop the service
2. **Ensure enabled state** — enable or disable at boot

For `restarted` and `reloaded`, the step produces a **single op**. These are
imperative one-shot actions — check always reports drift and execute always
fires. The `enabled` field is ignored for these states.

The `restarted` and `reloaded` states are typically used as `on_change`
targets, not as standalone steps — see [Reload pattern](#reload-pattern) below.

## Examples

### Start and enable

```scampi
posix.service {
  name    = "nginx"
  state   = posix.ServiceState.running
  enabled = true
}
```

### Stop and disable

```scampi
posix.service {
  name    = "apache2"
  state   = posix.ServiceState.stopped
  enabled = false
}
```

### Reload pattern

The most common use of `reloaded` is as an `on_change` target — bind it to a
name with `let`, then reference it from a step that updates the service's
config:

```scampi
let reload_nginx = posix.service {
  name  = "nginx"
  state = posix.ServiceState.reloaded
}

posix.copy {
  src       = posix.source_local { path = "./nginx.conf" }
  dest      = "/etc/nginx/nginx.conf"
  perm      = "0644"
  owner     = "root"
  group     = "root"
  on_change = [reload_nginx]
}

posix.service {
  name    = "nginx"
  state   = posix.ServiceState.running
  enabled = true
}
```

The reload only fires when the copy actually modifies the destination. The
final `running` step makes sure the service is up regardless.

If the init system doesn't support reload (e.g. launchctl), scampi
automatically falls back to a full restart.

### Restart unconditionally

```scampi
posix.service {
  name  = "nginx"
  state = posix.ServiceState.restarted
}
```

### Running but not at boot

```scampi
posix.service {
  desc    = "one-time migration service"
  name    = "migrate"
  state   = posix.ServiceState.running
  enabled = false
}
```
