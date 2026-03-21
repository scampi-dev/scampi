---
title: user
---

Ensure a user account exists or is absent on the target.

## Fields

| Field      | Type   | Required | Default     | Description                                 |
| ---------- | ------ | :------: | ----------- | ------------------------------------------- |
| `name`     | string |    ✓     |             | Username to manage                          |
| `desc`     | string |          |             | Human-readable description                  |
| `state`    | string |          | `"present"` | Desired state: `present` or `absent`        |
| `shell`    | string |          |             | Login shell                                 |
| `home`     | string |          |             | Home directory (useradd default if omitted) |
| `system`   | bool   |          | `false`     | Create as system user                       |
| `password` | string |          |             | Pre-hashed password                         |
| `groups`   | list   |          |             | Supplementary group names                   |

## States

| State     | Behavior                                                                                                                                       |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `present` | Create the user if missing. If the user exists but shell, home, or groups differ from the desired state, modify the user to match. Idempotent. |
| `absent`  | Delete the user if it exists. No-op if already absent.                                                                                         |

## How it works

For `present`, the step checks whether the user exists and compares the current
shell, home directory, and supplementary groups against the desired values.
Drift is reported per-field. If the user doesn't exist, it's created with
`useradd`. If it exists but properties differ, it's modified with `usermod`.

For `absent`, the step checks whether the user exists and removes it with
`userdel` if so.

The `password` field accepts a pre-hashed password string (as produced by
`openssl passwd` or `mkpasswd`). Use `secret()` to avoid storing hashes in
plain text.

## Examples

### Create a user

```python
user(
    name="hal9000",
    shell="/bin/bash",
    home="/home/hal9000",
    groups=["sudo", "docker"],
)
```

### System user

```python
user(
    desc="application daemon",
    name="appd",
    system=True,
    shell="/usr/sbin/nologin",
)
```

### Create a user for later steps

Users created by a `user` step can be referenced as `owner` in later steps.
During `scampi check`, the engine defers "unknown user" errors when the user
is promised by an earlier step.

```python
user(name="appd", system=True, shell="/usr/sbin/nologin")
dir(path="/opt/app", owner="appd", group="appd")
```

### Remove a user

```python
user(name="olduser", state="absent")
```

### User with password

```python
user(
    name="deploy",
    shell="/bin/bash",
    password=secret("deploy_password_hash"),
)
```
