---
title: service
weight: 4
---

Manage service state: running, stopped, restarted, or reloaded.
Works with systemd, OpenRC, and launchctl.

## Fields

| Field     | Type   | Required | Default      | Description |
|-----------|--------|:--------:|--------------|-------------|
| `name`    | string | ✓ |              | Service name |
| `desc`    | string |   |              | Human-readable description |
| `enabled` | bool   |   | `true`       | Whether the service should start at boot |
| `state`   | string |   | `"running"`  | Desired state (see below) |

## States

| State | Behavior |
|-------|----------|
| `running` | Start the service if not active. Idempotent. |
| `stopped` | Stop the service if active. Idempotent. |
| `restarted` | Restart the service unconditionally. Always fires. |
| `reloaded` | Reload the service unconditionally. Falls back to restart if the init system doesn't support reload. Always fires. |

## How it works

For `running` and `stopped`, the step produces two independent ops that run in
parallel:

1. **Ensure active state** — start or stop the service
2. **Ensure enabled state** — enable or disable at boot

For `restarted` and `reloaded`, the step produces a **single op**. These are
imperative one-shot actions — check always reports drift and execute always
fires. The `enabled` field is ignored for these states.

## Examples

### Start and enable

```python
service(name="nginx", state="running", enabled=True)
```

### Stop and disable

```python
service(name="apache2", state="stopped", enabled=False)
```

### Restart

```python
service(name="nginx", state="restarted")
```

### Reload (with automatic restart fallback)

```python
service(name="nginx", state="reloaded")
```

If the init system doesn't support reload (e.g. launchctl), scampi
automatically falls back to a full restart.

### Running but not at boot

```python
service(
    desc = "one-time migration service",
    name = "migrate",
    state = "running",
    enabled = False,
)
```
