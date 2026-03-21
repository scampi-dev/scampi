---
title: Concepts
weight: 2
---

Scampi has a small set of core concepts. Understanding them makes everything else
click.

## The mental model

```
Starlark config → Steps → Actions → Ops → Target
```

You write **steps** in Starlark. The engine plans **actions** from those steps,
breaks them into **ops**, and executes those ops against a **target**.

## Steps

A **step** is a declarative work item in your Starlark config. It says *what*
you want, not *how* to get there:

```python
pkg(packages=["nginx"], state="present", source=system())
```

Each step has a **kind** (`pkg`, `copy`, `service`, etc.) that determines which
Go handler (called a **step type**) processes it. See the
[Step Reference]({{< relref "../steps" >}}) for all built-in kinds.

## Actions

An **action** is the planned execution of one step. When scampi reads your
config, each step becomes an action in the execution plan. Actions execute
sequentially in the order you declared them.

## Ops

An **op** is the smallest executable unit. A single action may produce multiple
ops. For example, a `copy` step produces:

1. A file copy op
2. A permission op (depends on #1)
3. An ownership op (depends on #1)

Ops within an action form a DAG (directed acyclic graph) and run in parallel
where their dependencies allow. Every op implements the **check/execute**
pattern:

- **Check**: inspect current state, return whether a change is needed
- **Execute**: make the change (only runs if check says so)

This is what makes scampi idempotent. Running `apply` when reality already
matches your config is a no-op.

## Sources

A **source** tells a step where its content comes from. Scampi separates
*where content comes from* (source resolvers) from *what to do with it* (steps).
The two compose independently:

| Resolver   | Description                       |
| ---------- | --------------------------------- |
| `local()`  | File on the local machine         |
| `inline()` | String literal embedded in config |
| `remote()` | URL fetched via HTTP/HTTPS        |

Every step that accepts a `src` field works with every source resolver. You don't
need a different step to download a file vs. copy a local one — the step declares
*what* the desired state is, and the source resolver handles *where* the content
comes from:

```python
copy(src=remote(url="https://example.com/config"), ...)
template(src=local("./nginx.conf.tmpl"), ...)
unarchive(src=remote(url="https://github.com/.../v1.0.tar.gz"), ...)
copy(src=inline("nameserver 1.1.1.1\n"), ...)
```

This is a deliberate design choice. Steps and sources scale independently —
adding a new source type (like `git()` or `s3()`) automatically works with every
existing step, and adding a new step that reads content automatically works with
every existing source. No combinatorial explosion, no special cases.

There is no `fetch` step because none is needed. `copy(src=remote(...))` already
downloads a file to a path — and gets caching, checksums, idempotency, ownership,
and permission management for free. A `fetch` step would just be `copy` with
fewer knobs.

## Source machine and targets

Scampi distinguishes between two sides:

- **Source side**: where scampi runs and where your configs, templates, secrets,
  and cached downloads live.
- **Target side**: where ops execute — the system being converged.

With `target.ssh(...)`, these are different machines. With `target.local()`
(the default), they're the same machine — but the engine still treats them as
separate concerns internally: source access reads configs and caches data,
target access performs convergence mutations.

Targets expose **capabilities** that describe what they can do: filesystem
operations, package management, service control, etc.

Steps declare what capabilities they need. If a target doesn't have the right
capabilities, scampi fails fast with a clear error before executing anything.

```python
target.ssh(name="web", host="app.example.com", user="deploy")
```

See [Configuration]({{< relref "../configuration" >}}) for target setup details.

## Plans

Before executing anything, scampi builds a **plan** — the full set of actions
for a deploy block. You can inspect plans with three commands:

- `scampi plan` — show the execution plan without touching the target
- `scampi check` — run the plan's checks to see what would change
- `scampi apply` — execute the plan and converge the target

## Convergence

Scampi is a convergence engine. Each run compares desired state (your config)
against actual state (what's on the target) and makes the minimum changes needed
to close the gap. If there's no gap, nothing happens.

This means you can run scampi repeatedly — after a reboot, after a manual
change, after a deploy — and it always brings the system back to your declared
state.
