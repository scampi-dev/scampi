---
title: Project Layout
weight: 2
---

Scampi doesn't impose a directory structure, but here are some patterns that work
well as projects grow.

## Minimal

A single file is enough:

{{< filetree/container >}}
  {{< filetree/file name="deploy.star" >}}
  {{< filetree/file name="nginx.conf" >}}
{{< /filetree/container >}}

```python {filename="deploy.star"}
target.local(name="dev")

deploy(
    name = "webserver",
    steps = [
        pkg(packages=["nginx"], state="present", source=system()),
        copy(src=local("nginx.conf"), dest="/etc/nginx/nginx.conf", perm="0644"),
        service(name="nginx", state="running", enabled=True),
    ],
)
```

```nginx {filename="nginx.conf"}
worker_processes auto;

events {
    worker_connections 1024;
}

http {
    server {
        listen 80;
        root /var/www/html;
    }
}
```

## Small

Separate targets from deploy logic:

{{< filetree/container >}}
  {{< filetree/file name="targets.star" >}}
  {{< filetree/file name="deploy.star" >}}
  {{< filetree/folder name="files" >}}
    {{< filetree/file name="nginx.conf" >}}
    {{< filetree/file name="app.env" >}}
  {{< /filetree/folder >}}
  {{< filetree/folder name="templates" >}}
    {{< filetree/file name="Caddyfile.tmpl" >}}
  {{< /filetree/folder >}}
{{< /filetree/container >}}

```python {filename="targets.star"}
target.ssh(name="web", host="app.example.com", user="deploy")
```

```python {filename="deploy.star"}
load("targets.star", "web")

deploy(
    name = "app",
    targets = ["web"],
    steps = [
        pkg(packages=["caddy"], state="present", source=system()),
        copy(src=local("files/app.env"), dest="/opt/app/.env", perm="0600", owner="app", group="app"),
        template(
            src = local("templates/Caddyfile.tmpl"),
            dest = "/etc/caddy/Caddyfile",
            perm = "0644",
            data = {"domain": "app.example.com"},
        ),
        service(name="caddy", state="running", enabled=True),
    ],
)
```

```text {filename="files/app.env"}
NODE_ENV=production
PORT=3000
```

```text {filename="templates/Caddyfile.tmpl"}
{{ .domain }} {
    reverse_proxy localhost:3000
}
```

## Medium

Group by concern when managing multiple services:

{{< filetree/container >}}
  {{< filetree/file name="targets.star" >}}
  {{< filetree/file name="web.star" >}}
  {{< filetree/file name="db.star" >}}
  {{< filetree/file name="monitoring.star" >}}
  {{< filetree/folder name="files" >}}
    {{< filetree/file name="nginx.conf" >}}
    {{< filetree/file name="pg_hba.conf" >}}
  {{< /filetree/folder >}}
  {{< filetree/folder name="templates" >}}
    {{< filetree/file name="prometheus.yml.tmpl" >}}
  {{< /filetree/folder >}}
{{< /filetree/container >}}

```python {filename="targets.star"}
target.ssh(name="web", host="web.example.com", user="deploy")
target.ssh(name="db", host="db.example.com", user="deploy")
target.ssh(name="mon", host="mon.example.com", user="deploy")
```

```python {filename="web.star"}
load("targets.star", "web")

deploy(
    name = "web",
    targets = ["web"],
    steps = [
        pkg(packages=["nginx", "certbot"], state="present", source=system()),
        copy(src=local("files/nginx.conf"), dest="/etc/nginx/nginx.conf", perm="0644"),
        service(name="nginx", state="running", enabled=True),
    ],
)
```

```python {filename="db.star"}
load("targets.star", "db")

deploy(
    name = "database",
    targets = ["db"],
    steps = [
        pkg(packages=["postgresql-16"], state="present", source=system()),
        copy(src=local("files/pg_hba.conf"), dest="/etc/postgresql/16/main/pg_hba.conf", perm="0640", owner="postgres", group="postgres"),
        service(name="postgresql", state="running", enabled=True),
    ],
)
```

```python {filename="monitoring.star"}
load("targets.star", "mon")

deploy(
    name = "monitoring",
    targets = ["mon"],
    steps = [
        pkg(packages=["prometheus", "grafana"], state="present", source=system()),
        template(
            src = local("templates/prometheus.yml.tmpl"),
            dest = "/etc/prometheus/prometheus.yml",
            perm = "0644",
            data = {"targets": ["web.example.com", "db.example.com"]},
        ),
        service(name="prometheus", state="running", enabled=True),
        service(name="grafana-server", state="running", enabled=True),
    ],
)
```

## Large

Split into directories per environment. Use Starlark functions to define steps
once and vary the data per environment:

{{< filetree/container >}}
  {{< filetree/folder name="shared" >}}
    {{< filetree/file name="targets.star" >}}
    {{< filetree/file name="web.star" >}}
  {{< /filetree/folder >}}
  {{< filetree/file name="production.star" >}}
  {{< filetree/file name="staging.star" >}}
  {{< filetree/folder name="templates" >}}
    {{< filetree/file name="nginx.conf.tmpl" >}}
  {{< /filetree/folder >}}
{{< /filetree/container >}}

```python {filename="shared/targets.star"}
target.ssh(name="prod-web", host="web.prod.example.com", user="deploy")
target.ssh(name="staging-web", host="web.staging.example.com", user="deploy")
```

```python {filename="shared/web.star"}
def web_steps(domain):
    return [
        pkg(packages=["nginx"], state="present", source=system()),
        template(
            src = local("templates/nginx.conf.tmpl"),
            dest = "/etc/nginx/nginx.conf",
            perm = "0644",
            data = {"values": {"domain": domain, "upstream_port": 3000}},
        ),
        service(name="nginx", state="running", enabled=True),
    ]
```

```python {filename="production.star"}
load("shared/targets.star", "prod-web")
load("shared/web.star", "web_steps")

deploy(name="prod-web", targets=["prod-web"], steps=web_steps("prod.example.com"))
```

```python {filename="staging.star"}
load("shared/targets.star", "staging-web")
load("shared/web.star", "web_steps")

deploy(name="staging-web", targets=["staging-web"], steps=web_steps("staging.example.com"))
```

The production and staging files are pure wiring — the actual step logic lives in
one place. Use `load()` to share target definitions and step functions across
files.
