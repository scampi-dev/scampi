---
title: template
---

Render Go templates with data substitution and deploy the result with owner and
permission management.

## Fields

| Field    | Type   | Required | Description                                                           |
|----------|--------|:--------:|-----------------------------------------------------------------------|
| `src`    | source |    ✓     | [Source resolver]({{< relref "../configuration#source-resolvers" >}}) |
| `dest`   | string |    ✓     | Output file path (on target)                                          |
| `group`  | string |    ✓     | Group name or GID                                                     |
| `owner`  | string |    ✓     | Owner user name or UID                                                |
| `perm`   | string |    ✓     | File permissions                                                      |
| `data`   | dict   |          | Data sources for template rendering                                   |
| `desc`   | string |          | Human-readable description                                            |
| `verify` | string |          | Command to validate content before writing (`%s` = temp file path)    |

### Source resolvers

The `src` field accepts a source resolver:

- **`local("./path")`** — reads a template file from the local machine
- **`inline("content")`** — uses the given string as template content
- **`remote(url="...")`** — downloads a template via HTTP/HTTPS (with optional
  `checksum` for verification)

See [Source resolvers]({{< relref "../configuration#source-resolvers" >}}) for
full details.

### Data fields

The `data` dict supports:

| Key      | Type                 | Description                                          |
|----------|----------------------|------------------------------------------------------|
| `values` | dict (string→any)    | Arbitrary key-value pairs accessible in the template |
| `env`    | dict (string→string) | Map template variable names to environment variables |

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

```python
template(
    src = local("./templates/nginx.conf.tmpl"),
    dest = "/etc/nginx/nginx.conf",
    perm = "0644",
    owner = "root",
    group = "root",
    data = {
        "values": {
            "server_name": "app.example.com",
            "upstream_port": 8080,
        },
    },
)
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

Starlark supports triple-quoted strings for multi-line content:

```python
template(
    src = inline("""\
[Service]
Environment=DB_HOST={{ .values.db_host }}
Environment=DB_PORT={{ .values.db_port }}
Environment=DB_NAME={{ .values.db_name }}
"""),
    dest = "/etc/systemd/system/app.service.d/env.conf",
    perm = "0644",
    owner = "root",
    group = "root",
    data = {"values": {
        "db_host": "db.internal",
        "db_port": 5432,
        "db_name": "myapp",
    }},
)
```

### With environment variables

```python
template(
    src = local("./app.env.tmpl"),
    dest = "/opt/app/.env",
    perm = "0600",
    owner = "app",
    group = "app",
    data = {
        "env": {"db_password": "DB_PASSWORD"},
        "values": {"app_name": "myapp"},
    },
)
```

In the template, environment variables are accessible under `.env`:

```text {filename="app.env.tmpl"}
APP_NAME={{ .values.app_name }}
DB_PASSWORD={{ .env.db_password }}
```

### Remote template

```python
template(
    src = remote(url="https://example.com/templates/nginx.conf.tmpl"),
    dest = "/etc/nginx/nginx.conf",
    perm = "0644",
    owner = "root",
    group = "root",
    data = {
        "values": {
            "server_name": "app.example.com",
            "upstream_port": 8080,
        },
    },
)
```

### With verify

```python
template(
    src = local("./templates/nginx.conf.tmpl"),
    dest = "/etc/nginx/nginx.conf",
    perm = "0644",
    owner = "root",
    group = "root",
    data = {
        "values": {
            "server_name": "app.example.com",
            "upstream_port": 8080,
        },
    },
    verify = "nginx -t -c %s",
)
```
