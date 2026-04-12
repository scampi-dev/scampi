---
title: Project Layout
weight: 2
---

scampi doesn't impose a directory structure, but here are some patterns that work
well as projects grow.

## Minimal

A single file is enough:

{{< filetree/container >}}
  {{< filetree/file name="deploy.scampi" >}}
  {{< filetree/file name="nginx.conf" >}}
{{< /filetree/container >}}

```scampi {filename="deploy.scampi"}
module main

import "std"
import "std/local"
import "std/posix"

let dev = local.target { name = "dev" }

std.deploy(name = "webserver", targets = [dev]) {
  posix.pkg {
    packages = ["nginx"]
    source   = posix.pkg_system {}
  }

  posix.copy {
    src   = posix.source_local { path = "./nginx.conf" }
    dest  = "/etc/nginx/nginx.conf"
    perm  = "0644"
    owner = "root"
    group = "root"
  }

  posix.service { name = "nginx", state = posix.ServiceState.running, enabled = true }
}
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

Separate targets from deploy logic using the module system. Your project needs
a `scampi.mod` to declare its module path:

{{< filetree/container >}}
  {{< filetree/file name="scampi.mod" >}}
  {{< filetree/file name="deploy.scampi" >}}
  {{< filetree/file name="targets.scampi" >}}
  {{< filetree/folder name="files" >}}
    {{< filetree/file name="nginx.conf" >}}
    {{< filetree/file name="app.env" >}}
  {{< /filetree/folder >}}
  {{< filetree/folder name="templates" >}}
    {{< filetree/file name="Caddyfile.tmpl" >}}
  {{< /filetree/folder >}}
{{< /filetree/container >}}

```text {filename="scampi.mod"}
module example.com/my-infra
```

```scampi {filename="targets.scampi"}
module targets

import "std/ssh"

let web = ssh.target { name = "web", host = "app.example.com", user = "deploy" }
```

```scampi {filename="deploy.scampi"}
module main

import "std"
import "std/posix"
import "example.com/my-infra/targets"

std.deploy(name = "app", targets = [targets.web]) {
  posix.pkg {
    packages = ["caddy"]
    source   = posix.pkg_system {}
  }

  posix.copy {
    src   = posix.source_local { path = "./files/app.env" }
    dest  = "/opt/app/.env"
    perm  = "0600"
    owner = "app"
    group = "app"
  }

  posix.template {
    src  = posix.source_local { path = "./templates/Caddyfile.tmpl" }
    dest = "/etc/caddy/Caddyfile"
    perm = "0644", owner = "root", group = "root"
    data = {"values": {"domain": "app.example.com"}}
  }

  posix.service { name = "caddy", state = posix.ServiceState.running, enabled = true }
}
```

The entry file declares `module main` and imports the targets module using the
full path from `scampi.mod`. The leaf segment (`targets`) becomes the namespace
— so `targets.web` refers to the `web` binding from `targets.scampi`.

## Medium

Group by concern. Use a directory module when a concern has multiple files —
all files in a directory declaring the same `module` name get merged into one
namespace:

{{< filetree/container >}}
  {{< filetree/file name="scampi.mod" >}}
  {{< filetree/file name="deploy.scampi" >}}
  {{< filetree/file name="targets.scampi" >}}
  {{< filetree/folder name="steps" >}}
    {{< filetree/file name="web.scampi" >}}
    {{< filetree/file name="db.scampi" >}}
    {{< filetree/file name="monitoring.scampi" >}}
  {{< /filetree/folder >}}
  {{< filetree/folder name="files" >}}
    {{< filetree/file name="nginx.conf" >}}
    {{< filetree/file name="pg_hba.conf" >}}
  {{< /filetree/folder >}}
  {{< filetree/folder name="templates" >}}
    {{< filetree/file name="prometheus.yml.tmpl" >}}
  {{< /filetree/folder >}}
{{< /filetree/container >}}

```scampi {filename="targets.scampi"}
module targets

import "std/ssh"

let web = ssh.target { name = "web", host = "web.example.com", user = "deploy" }
let db  = ssh.target { name = "db", host = "db.example.com", user = "deploy" }
let mon = ssh.target { name = "mon", host = "mon.example.com", user = "deploy" }
```

```scampi {filename="steps/web.scampi"}
module steps

import "std"
import "std/posix"

decl web_server() std.Step {
  posix.pkg { packages = ["nginx", "certbot"], source = posix.pkg_system {} }
  posix.copy {
    src   = posix.source_local { path = "./files/nginx.conf" }
    dest  = "/etc/nginx/nginx.conf"
    perm  = "0644", owner = "root", group = "root"
  }
  posix.service { name = "nginx", state = posix.ServiceState.running, enabled = true }
}
```

```scampi {filename="deploy.scampi"}
module main

import "std"
import "example.com/my-infra/targets"
import "example.com/my-infra/steps"

std.deploy(name = "web", targets = [targets.web]) {
  steps.web_server {}
}

std.deploy(name = "database", targets = [targets.db]) {
  steps.db_setup {}
}

std.deploy(name = "monitoring", targets = [targets.mon]) {
  steps.monitoring_stack {}
}
```

All three files in `steps/` declare `module steps`. scampi merges them into one
namespace, so `steps.web_server`, `steps.db_setup`, and
`steps.monitoring_stack` are all available from a single `import`.
