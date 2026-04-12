---
title: template
---

Render Go templates with data substitution and deploy the result with owner and
permission management.

## Fields

| Field       | Type        | Required | Description                                                             |
| ----------- | ----------- | :------: | ----------------------------------------------------------------------- |
| `src`       | `Source`    |    ✓     | Source resolver — see [below](#source-resolvers)                        |
| `dest`      | string      |    ✓     | Output file path on target (must be absolute, validated by `@std.path`) |
| `perm`      | string      |    ✓     | File permissions (`@std.filemode`)                                      |
| `owner`     | string      |    ✓     | Owner user name or UID (`@std.nonempty`)                                |
| `group`     | string      |    ✓     | Group name or GID (`@std.nonempty`)                                     |
| `data`      | any?        |          | Data sources for template rendering — see [below](#data-fields)         |
| `verify`    | string?     |          | Command to validate content before writing (`%s` = temp file path)      |
| `desc`      | string?     |          | Human-readable description                                              |
| `on_change` | list\[Step] |          | Steps to trigger when this template modifies the destination            |

### Source resolvers

The `src` field accepts a `posix.Source` from one of three resolvers:

- **`posix.source_local { path = "./path" }`** — reads a template file from the
  local machine
- **`posix.source_inline { content = "..." }`** — uses the given string as
  template content
- **`posix.source_remote { url = "https://...", checksum = "..." }`** —
  downloads via HTTP/HTTPS, with optional checksum verification

### Data fields

The `data` value supports two top-level keys:

| Key      | Type                | Description                                          |
| -------- | ------------------- | ---------------------------------------------------- |
| `values` | map[string, any]    | Arbitrary key-value pairs accessible in the template |
| `env`    | map[string, string] | Map template variable names to environment variables |

## How it works

The `template` step uses Go's `text/template` syntax. It renders the template
with provided data, compares the result against the existing file on the target,
and writes it only if different.

Like `copy`, it produces a dependency chain: render first, then set permissions
and ownership in parallel.

### Verify

When `verify` is set, the rendered content is written to a temporary file first.
The verify command runs with `%s` replaced by the temp file path. If the command
exits 0, the content is written to the destination. If it exits non-zero, the
destination is left untouched and the step fails. The temp file is always
cleaned up.

Verify only runs when the content actually needs to change — idempotent runs
skip it entirely.

## Examples

### From a file

```scampi
posix.template {
  src   = posix.source_local { path = "./templates/nginx.conf.tmpl" }
  dest  = "/etc/nginx/nginx.conf"
  perm  = "0644"
  owner = "root"
  group = "root"
  data  = {
    "values": {
      "server_name":   "app.example.com",
      "upstream_port": 8080,
    },
  }
}
```

With the template file:

```text {filename="templates/nginx.conf.tmpl"}
server {
    listen 80;
    server_name {{ .values.server_name }};

    location / {
        proxy_pass http://127.0.0.1:{{ .values.upstream_port }};
    }
}
```

### Inline template

scampi supports multi-line strings for inline content:

```scampi
posix.template {
  src = posix.source_inline {
    content = "[Service]\nEnvironment=DB_HOST={{ .values.db_host }}\nEnvironment=DB_PORT={{ .values.db_port }}\nEnvironment=DB_NAME={{ .values.db_name }}\n"
  }
  dest  = "/etc/systemd/system/app.service.d/env.conf"
  perm  = "0644"
  owner = "root"
  group = "root"
  data  = {
    "values": {
      "db_host": "db.internal",
      "db_port": 5432,
      "db_name": "myapp",
    },
  }
}
```

### With environment variables

```scampi
posix.template {
  src   = posix.source_local { path = "./app.env.tmpl" }
  dest  = "/opt/app/.env"
  perm  = "0600"
  owner = "app"
  group = "app"
  data  = {
    "env":    {"db_password": "DB_PASSWORD"},
    "values": {"app_name": "myapp"},
  }
}
```

In the template, environment variables are accessible under `.env`:

```text {filename="app.env.tmpl"}
APP_NAME={{ .values.app_name }}
DB_PASSWORD={{ .env.db_password }}
```

### Remote template

```scampi
posix.template {
  src   = posix.source_remote { url = "https://example.com/templates/nginx.conf.tmpl" }
  dest  = "/etc/nginx/nginx.conf"
  perm  = "0644"
  owner = "root"
  group = "root"
  data  = {
    "values": {
      "server_name":   "app.example.com",
      "upstream_port": 8080,
    },
  }
}
```

### With verify and reload

```scampi
let reload_nginx = posix.service {
  name  = "nginx"
  state = posix.ServiceState.reloaded
}

posix.template {
  src       = posix.source_local { path = "./templates/nginx.conf.tmpl" }
  dest      = "/etc/nginx/nginx.conf"
  perm      = "0644"
  owner     = "root"
  group     = "root"
  data      = { "values": { "server_name": "app.example.com" } }
  verify    = "nginx -t -c %s"
  on_change = [reload_nginx]
}
```
