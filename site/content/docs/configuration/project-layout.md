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

## Growing

As a project grows, you'll want to separate concerns. scampi's module system
handles this — see [Modules]({{< relref "../modules" >}}) for how to write and
import your own modules.

Typical directory structure for a multi-target project:

{{< filetree/container >}}
  {{< filetree/file name="scampi.mod" >}}
  {{< filetree/file name="deploy.scampi" >}}
  {{< filetree/folder name="files" >}}
    {{< filetree/file name="nginx.conf" >}}
    {{< filetree/file name="app.env" >}}
  {{< /filetree/folder >}}
  {{< filetree/folder name="templates" >}}
    {{< filetree/file name="Caddyfile.tmpl" >}}
  {{< /filetree/folder >}}
{{< /filetree/container >}}

The entry point is always a `.scampi` file with `module main`. Templates and
static files sit alongside it in the project directory.
