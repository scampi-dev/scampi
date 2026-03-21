---
title: group
---

Ensure a group exists or is absent on the target.

## Fields

| Field    | Type   | Required | Default     | Description                          |
| -------- | ------ | :------: | ----------- | ------------------------------------ |
| `name`   | string |    ✓     |             | Group name to manage                 |
| `desc`   | string |          |             | Human-readable description           |
| `state`  | string |          | `"present"` | Desired state: `present` or `absent` |
| `gid`    | int    |          |             | Group ID (auto-assigned if omitted)  |
| `system` | bool   |          | `false`     | Create as system group               |

## States

| State     | Behavior                                                        |
| --------- | --------------------------------------------------------------- |
| `present` | Create the group if it doesn't exist. No-op if already present. |
| `absent`  | Delete the group if it exists. No-op if already absent.         |

## How it works

For `present`, the step checks whether the group exists. If not, it creates it
with `groupadd`. If the group already exists, nothing happens.

For `absent`, the step checks whether the group exists and removes it with
`groupdel` if so.

## Examples

### Create a group

```python
group(name="appusers", gid=1100)
```

### System group

```python
group(
    desc="application service group",
    name="appd",
    system=True,
)
```

### Create a group for later steps

Groups created by a `group` step can be referenced as `group` in later steps.
During `scampi check`, the engine defers "unknown group" errors when the group
is promised by an earlier step.

```python
group(name="appusers")
dir(path="/opt/app", group="appusers")
```

### Remove a group

```python
group(name="oldgroup", state="absent")
```
