---
title: Examples
linkTitle: Examples
weight: 3
description: What a scampi config looks like, and the steps you can use.
type: docs
---

A scampi config is a `module main` that imports the standard library,
declares one or more targets, and lists the desired state inside a
`std.deploy` block. The engine checks each step and only acts when
reality doesn't already match.

## A first config

Declare a directory, a versioned subdirectory, and a symlink that
points at it. Run it twice — the second run makes no changes.

```scampi
module main

import "std"
import "std/local"
import "std/posix"

let machine = local.target { name = "my-machine" }

std.deploy(name = "hello", targets = [machine]) {
  posix.dir { path = "/tmp/scampi-demo", perm = "0755" }
  posix.dir { path = "/tmp/scampi-demo/v1", perm = "0755" }

  posix.symlink {
    target = "/tmp/scampi-demo/v1"
    link   = "/tmp/scampi-demo/current"
  }
}
```

## Files, packages, and services

A more realistic slice: install packages, render a config file from a
template, and keep a service running.

```scampi
module main

import "std"
import "std/local"
import "std/posix"

let host = local.target { name = "web" }

std.deploy(name = "web", targets = [host]) {
  posix.pkg {
    packages = ["nginx"]
    source   = posix.pkg_system {}
  }

  posix.template {
    src   = posix.source_local { path = "./nginx.conf.tmpl" }
    dest  = "/etc/nginx/nginx.conf"
    perm  = "0644"
    owner = "root"
    group = "root"
  }

  posix.service {
    name = "nginx"
  }
}
```

## Running against a remote host

Swap the target for an SSH target — the steps stay identical. The
engine reads and writes over SSH instead of the local filesystem.

```scampi
import "std/ssh"

let host = ssh.target {
  name = "web1"
  host = "web1.example.com"
  user = "deploy"
}
```

## Available steps

Every step is built in — no plugins to install.

| Step                 | Does                                                        |
| -------------------- | ----------------------------------------------------------- |
| `posix.copy`         | Copy files with owner and permission management             |
| `posix.dir`          | Ensure a directory exists with optional perms and ownership |
| `posix.template`     | Render templates with data substitution                     |
| `posix.symlink`      | Create and manage symbolic links                            |
| `posix.unarchive`    | Extract an archive to a target directory                    |
| `posix.pkg`          | Ensure packages are present, absent, or at the latest       |
| `posix.service`      | Manage service state: running, stopped, restarted, reloaded |
| `posix.user`         | Ensure a user account exists or is absent                   |
| `posix.group`        | Ensure a group exists or is absent                          |
| `posix.mount`        | Manage filesystem mounts and fstab entries                  |
| `posix.sysctl`       | Manage kernel parameters via sysctl                         |
| `posix.firewall`     | Manage firewall rules via UFW or firewalld                  |
| `posix.run`          | Run a shell command with an optional idempotency check      |
| `posix.run_set`      | Shell-driven set reconciliation (list / add / remove)       |
| `container.instance` | Manage container lifecycle: running, stopped, or absent     |

## Targets

| Target         | Runs against               |
| -------------- | -------------------------- |
| `local.target` | the machine scampi runs on |
| `ssh.target`   | a remote host over SSH     |

Source values like `posix.source_local { path = ... }` and
`posix.source_inline { content = ... }` compose into the file steps,
so the same step works whether the bytes come from disk or inline.
