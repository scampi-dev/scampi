---
title: user
---

Ensure a user account exists or is absent on the target.

## Fields

| Field       | Type           | Required | Default             | Description                                 |
| ----------- | -------------- | :------: | ------------------- | ------------------------------------------- |
| `name`      | string         |    ✓     |                     | Username to manage (`@std.nonempty`)        |
| `state`     | `UserState`    |          | `UserState.present` | Desired state                               |
| `shell`     | string?        |          |                     | Login shell                                 |
| `home`      | string?        |          |                     | Home directory (useradd default if omitted) |
| `system`    | bool?          |          |                     | Create as system user                       |
| `password`  | string?        |          |                     | Pre-hashed password                         |
| `groups`    | list\[string]? |          |                     | Supplementary group names                   |
| `desc`      | string?        |          |                     | Human-readable description                  |
| `on_change` | list\[Step]    |          |                     | Steps to trigger when this user changes     |

## States

`posix.UserState` is an enum:

| Value               | Behavior                                                                                                                                       |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `UserState.present` | Create the user if missing. If the user exists but shell, home, or groups differ from the desired state, modify the user to match. Idempotent. |
| `UserState.absent`  | Delete the user if it exists. No-op if already absent.                                                                                         |

## How it works

For `present`, the step checks whether the user exists and compares the current
shell, home directory, and supplementary groups against the desired values.
Drift is reported per-field. If the user doesn't exist, it's created with
`useradd`. If it exists but properties differ, it's modified with `usermod`.

For `absent`, the step checks whether the user exists and removes it with
`userdel` if so.

The `password` field accepts a pre-hashed password string (as produced by
`openssl passwd` or `mkpasswd`). Use `std.secret(...)` to avoid storing hashes
in plain text.

## Examples

### Create a user

```scampi
posix.user {
  name   = "hal9000"
  shell  = "/bin/bash"
  home   = "/home/hal9000"
  groups = ["sudo", "docker"]
}
```

### System user

```scampi
posix.user {
  desc   = "application daemon"
  name   = "appd"
  system = true
  shell  = "/usr/sbin/nologin"
}
```

### Create a user for later steps

Users created by a `user` step can be referenced as `owner` in later steps.
During `scampi check`, the engine defers "unknown user" errors when the user
is promised by an earlier step.

```scampi
posix.user { name = "appd", system = true, shell = "/usr/sbin/nologin" }
posix.dir { path = "/opt/app", owner = "appd", group = "appd" }
```

### Remove a user

```scampi
posix.user { name = "olduser", state = posix.UserState.absent }
```

### User with password from secret store

```scampi
posix.user {
  name     = "deploy"
  shell    = "/bin/bash"
  password = std.secret("deploy_password_hash")
}
```
