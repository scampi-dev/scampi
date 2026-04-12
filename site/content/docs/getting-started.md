---
title: Getting Started
weight: 1
---

This guide walks you through installing scampi and writing your first
configuration.

## Install

See the [Install page]({{< relref "/get" >}}) for all options (one-liner, `go install`, manual download, build from source).

## Your first config

{{% steps %}}

### Create a config file

```scampi {filename="deploy.scampi"}
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

A scampi config starts with `module main` and the imports it needs. `std` is
the core, `std/local` gives you the local target, `std/posix` gives you the
POSIX steps.

### Check what would change

Dry-run to see what scampi would do without making changes:

```bash
scampi check deploy.scampi
```

Scampi inspects the current state and reports what differs from your declared
config. Yellow means something would change. Green means it already matches.

### Apply

```bash
scampi apply deploy.scampi
```

Run it again — everything should be green. That's convergence: the system already
matches your declared state, so there's nothing to do.

{{% /steps %}}

## Add a remote target

To manage a remote machine over SSH, swap the target module:

```scampi {filename="remote.scampi"}
module main

import "std"
import "std/ssh"
import "std/posix"

let web = ssh.target {
  name = "web"
  host = "192.168.1.10"
  user = "deploy"
}

std.deploy(name = "hello", targets = [web]) {
  posix.dir { path = "/tmp/scampi-demo", perm = "0755" }

  posix.copy {
    src   = posix.source_local { path = "./message.txt" }
    dest  = "/tmp/scampi-demo/message.txt"
    perm  = "0644"
    owner = "root"
    group = "root"
  }
}
```

Scampi connects via SSH — no agents or daemons needed on the remote host.

## Reading the syntax

The two main call patterns above:

- **`local.target { … }`** and **`posix.dir { … }`** — these are *decl calls*
  (struct literal syntax). Field assignments inside braces, separators are
  flexible. Used for steps, targets, and source resolvers.
- **`std.deploy(name = …, targets = [machine]) { … }`** — this is a *function
  call with a trailing block*. The function returns a `block[Deploy]`, and the
  `{ … }` after the parens is the deploy body.

If those patterns look unfamiliar, the [Language guide]({{< relref "language" >}})
walks through them in depth.

## Next steps

- Read [the Language guide]({{< relref "language" >}}) for a full tour of scampi's syntax
- Read [Concepts]({{< relref "concepts" >}}) to understand the execution model
- See [Configuration]({{< relref "configuration" >}}) for project layout, variables, and deploy block patterns
- Browse the [Step Reference]({{< relref "steps" >}}) for all built-in step types
