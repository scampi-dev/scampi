---
title: group
---

Ensure a group exists or is absent on the target.

## Fields

| Field       | Type         | Required | Default              | Description                              |
| ----------- | ------------ | :------: | -------------------- | ---------------------------------------- |
| `name`      | string       |    ✓     |                      | Group name to manage (`@std.nonempty`)   |
| `state`     | `GroupState` |          | `GroupState.present` | Desired state                            |
| `gid`       | int?         |          |                      | Group ID (auto-assigned if omitted)      |
| `system`    | bool?        |          |                      | Create as system group                   |
| `desc`      | string?      |          |                      | Human-readable description               |
| `on_change` | list\[Step]  |          |                      | Steps to trigger when this group changes |

## States

`posix.GroupState` is an enum:

| Value                | Behavior                                                        |
| -------------------- | --------------------------------------------------------------- |
| `GroupState.present` | Create the group if it doesn't exist. No-op if already present. |
| `GroupState.absent`  | Delete the group if it exists. No-op if already absent.         |

## How it works

For `present`, the step checks whether the group exists. If not, it creates it
with `groupadd`. If the group already exists, nothing happens.

For `absent`, the step checks whether the group exists and removes it with
`groupdel` if so.

## Examples

### Create a group

```scampi
posix.group { name = "appusers", gid = 1100 }
```

### System group

```scampi
posix.group {
  desc   = "application service group"
  name   = "appd"
  system = true
}
```

### Create a group for later steps

Groups created by a `group` step can be referenced as `group` in later steps.
During `scampi check`, the engine defers "unknown group" errors when the group
is promised by an earlier step.

```scampi
posix.group { name = "appusers" }
posix.dir { path = "/opt/app", group = "appusers" }
```

### Remove a group

```scampi
posix.group { name = "oldgroup", state = posix.GroupState.absent }
```
