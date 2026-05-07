---
title: copy
---

Copy files or inline content to the target with owner and permission management.

## Fields

| Field       | Type        | Required | Description                                                                  |
| ----------- | ----------- | :------: | ---------------------------------------------------------------------------- |
| `src`       | `Source`    |    ✓     | Source resolver — see [below](#source-resolvers)                             |
| `dest`      | string      |    ✓     | Destination file path on target (must be absolute, validated by `@std.path`) |
| `perm`      | string      |    ✓     | File permissions — `0644`, `u=rw,g=r,o=r`, or `rw-r--r--` (`@std.filemode`)  |
| `owner`     | string      |    ✓     | Owner user name or UID (`@std.nonempty`)                                     |
| `group`     | string      |    ✓     | Group name or GID (`@std.nonempty`)                                          |
| `verify`    | string?     |          | Command to validate content before writing (`%s` = temp file path)           |
| `desc`      | string?     |          | Human-readable description                                                   |
| `on_change` | list\[Step] |          | Steps to trigger when this copy modifies the destination                     |

## Source resolvers

The `src` field accepts a `posix.Source` from one of four resolvers:

- **`posix.source_local { path = "./path" }`** — reads a file from the local
  machine relative to the scampi file
- **`posix.source_inline { content = "..." }`** — uses the given string as file
  content
- **`posix.source_remote { url = "https://...", checksum = "sha256:..." }`** —
  downloads via HTTP/HTTPS, with optional checksum verification
- **`posix.source_target { path = "/abs/path" }`** — reads from the target
  itself, useful when a previous step generated the file and you want to
  install it elsewhere with managed perms / owner. Symlink covers most
  cases; use this when you need two independent copies.

See [Concepts → Sources]({{< relref "../concepts#sources" >}}) for the design
rationale.

## How it works

The `copy` step produces three ops that form a dependency chain:

1. **Copy file** — copies the source to the destination (compares content bytes)
2. **Set permissions** — ensures file mode matches (depends on #1)
3. **Set ownership** — ensures owner and group match (depends on #1)

The permission and ownership ops only run after the file copy succeeds. If the
file content already matches and permissions and ownership are correct, nothing
happens.

### Verify

When `verify` is set, the content is written to a temporary file first. The
verify command runs with `%s` replaced by the temp file path. If the command
exits 0, the content is written to the destination. If it exits non-zero, the
destination is left untouched and the step fails. The temp file is always
cleaned up.

Verify only runs when the content actually needs to change — idempotent runs
skip it entirely.

## Examples

### Basic file copy

```scampi {filename="deploy.scampi"}
posix.copy {
  src   = posix.source_local { path = "./nginx.conf" }
  dest  = "/etc/nginx/nginx.conf"
  perm  = "0644"
  owner = "root"
  group = "root"
}
```

### Inline content

```scampi {filename="deploy.scampi"}
posix.copy {
  desc  = "passwordless sudo for automation user"
  src   = posix.source_inline { content = "hal9000 ALL=(ALL) NOPASSWD:ALL\n" }
  dest  = "/etc/sudoers.d/hal9000"
  perm  = "0440"
  owner = "root"
  group = "root"
}
```

### Application config (POSIX permission notation)

```scampi {filename="deploy.scampi"}
posix.copy {
  desc  = "deploy app config"
  src   = posix.source_local { path = "./config.yaml" }
  dest  = "/opt/myapp/config.yaml"
  perm  = "u=rw,g=r,o="
  owner = "myapp"
  group = "myapp"
}
```

### Validated sudoers file

```scampi {filename="deploy.scampi"}
posix.copy {
  src    = posix.source_inline { content = "hal9000 ALL=(ALL) NOPASSWD:ALL\n" }
  dest   = "/etc/sudoers.d/hal9000"
  perm   = "0440"
  owner  = "root"
  group  = "root"
  verify = "visudo -cf %s"
}
```

### Validated nginx config

```scampi {filename="deploy.scampi"}
posix.copy {
  src    = posix.source_local { path = "./nginx.conf" }
  dest   = "/etc/nginx/nginx.conf"
  perm   = "0644"
  owner  = "root"
  group  = "root"
  verify = "nginx -t -c %s"
}
```

### Reload on change

A common pattern: bind a reload step to a name, then reference it from
`on_change` on the step that writes the config:

```scampi {filename="deploy.scampi"}
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
```

The reload only fires when the copy actually modifies the destination. On
subsequent runs (when the file is already correct) the reload is skipped.

### Remote file

```scampi {filename="deploy.scampi"}
posix.copy {
  desc  = "download IP lookup config"
  src   = posix.source_remote { url = "https://example.com/config.yaml" }
  dest  = "/etc/app/config.yaml"
  perm  = "0644"
  owner = "root"
  group = "root"
}
```

### Remote file with checksum

```scampi {filename="deploy.scampi"}
posix.copy {
  src = posix.source_remote {
    url      = "https://example.com/ca-bundle.crt"
    checksum = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
  }
  dest  = "/etc/ssl/certs/ca-bundle.crt"
  perm  = "0644"
  owner = "root"
  group = "root"
}
```

### Restrictive permissions (ls-style notation)

```scampi {filename="deploy.scampi"}
posix.copy {
  src   = posix.source_local { path = "./ssl/server.key" }
  dest  = "/etc/ssl/private/server.key"
  perm  = "rw-------"
  owner = "root"
  group = "root"
}
```
