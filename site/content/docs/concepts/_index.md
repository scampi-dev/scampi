---
title: Concepts
weight: 2
---

scampi has a small set of core concepts. Understanding them makes everything
else click.

For the **language** itself — syntax, types, decls, the trailing-block pattern
— see [the Language guide]({{< relref "../language" >}}). This page covers the
**execution model**: what happens after scampi reads your config.

## The mental model

```
config → steps → plan → check/execute → target
```

You write **steps**. scampi plans them, checks what needs to change, and
executes only the necessary mutations against a **target**.

## Steps

A **step** is a declarative work item. It says *what* you want, not *how* to
get there:

```scampi
posix.pkg {
  packages = ["nginx"]
  source   = posix.pkg_system {}
}
```

Steps come from the standard library modules — `posix` for filesystem and
system operations, `container` for container management, `rest` for HTTP APIs.
You can also define your own steps in user modules. See the
[Step Reference]({{< relref "../steps" >}}) for everything the standard library
provides.

Steps declare what they **provide** (e.g. a directory path) and what they
**require** (e.g. a parent directory). The engine builds a dependency graph
from these declarations and reorders steps for correctness — independent
steps can run in parallel, dependent steps are sequenced automatically.
You write them in whatever order makes sense to read; the engine figures out
the execution order.

## Check and execute

Every step implements two phases:

- **Check**: inspect current state, determine whether a change is needed
- **Execute**: make the change (only runs if check says so)

This is what makes scampi idempotent. Running `apply` when reality already
matches your config is a no-op. The three CLI commands map directly to this
model:

- `scampi plan` — show what would happen, without touching the target
- `scampi check` — run checks against real state, report what would change
- `scampi apply` — run checks, then execute whatever needs changing

A single step may produce multiple operations internally. For example, a
`copy` step checks the file content, permissions, and ownership independently
— if only the permissions drifted, only the permission fix runs. Operations
within a step can run in parallel where their dependencies allow.

## Sources

A **source** tells a step where its content comes from. scampi separates
*where content comes from* (source resolvers) from *what to do with it* (steps).
The two compose independently.

The `posix` module ships three source resolvers:

| Resolver              | Description                       |
| --------------------- | --------------------------------- |
| `posix.source_local`  | File on the local machine         |
| `posix.source_inline` | String literal embedded in config |
| `posix.source_remote` | URL fetched via HTTP/HTTPS        |

Every step that accepts a `src` field works with every source resolver. You
don't need a different step to download a file vs. copy a local one — the step
declares *what* the desired state is, and the source resolver handles *where*
the content comes from:

```scampi
posix.copy {
  src  = posix.source_remote { url = "https://example.com/config" }
  dest = "/etc/app/config"
  perm = "0644", owner = "root", group = "root"
}

posix.template {
  src  = posix.source_local { path = "./nginx.conf.tmpl" }
  dest = "/etc/nginx/nginx.conf"
  perm = "0644", owner = "root", group = "root"
}

posix.copy {
  src  = posix.source_inline { content = "nameserver 1.1.1.1\n" }
  dest = "/etc/resolv.conf"
  perm = "0644", owner = "root", group = "root"
}
```

Steps and sources scale independently — adding a new source type automatically
works with every existing step, and adding a new step that reads content
automatically works with every existing source.

## Source and target

scampi distinguishes between two sides:

- **Source side**: where scampi runs and where your configs, templates, secrets,
  and cached downloads live.
- **Target side**: where mutations happen — the system being converged.

With `ssh.target { … }`, these are different machines. With `local.target { … }`,
they're the same machine — but the engine still treats them as separate
concerns. Source access reads configs and caches data. Target access performs
convergence mutations.

Targets advertise **capabilities** — filesystem, packages, services, etc.
Steps declare what capabilities they need. If there's a mismatch, scampi fails
fast with a clear error before executing anything.

## Convergence

scampi is a convergence engine. Each run compares desired state (your config)
against actual state (what's on the target) and makes the minimum changes needed
to close the gap. If there's no gap, nothing happens.

This means you can run scampi repeatedly — after a reboot, after a manual
change, after a deploy — and it always brings the system back to your declared
state.

## Where to next

- [Language guide]({{< relref "../language" >}}) — syntax, types, decls, trailing blocks
- [Configuration]({{< relref "../configuration" >}}) — deploy blocks, source resolvers, secrets
- [Testing]({{< relref "../testing" >}}) — mock targets and the declarative expect model
- [Step Reference]({{< relref "../steps" >}}) — every built-in step type
