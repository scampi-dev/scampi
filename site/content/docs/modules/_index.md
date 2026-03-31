---
title: Modules
weight: 4
---

Scampi modules are reusable Starlark packages distributed as git repositories.
A module exports functions that produce steps — the same functions you use in
deploy blocks.

## Quick start

Add a module to your project:

```text
scampi mod init codeberg.org/yourname/yourproject
scampi mod add codeberg.org/scampi-modules/npm
```

Use it in your config:

```starlark {filename="deploy.scampi"}
load("codeberg.org/scampi-modules/npm", "proxy_host", "certificate")

target.local(name = "server")

deploy(
    name = "reverse-proxy",
    targets = ["server"],
    steps = [
        certificate("*.example.com"),
        proxy_host("example.com", forward_host = "localhost", forward_port = 3000),
    ],
)
```

## How it works

1. `scampi.mod` declares your module path and dependencies
2. `scampi mod add` fetches modules from git repositories into a local cache
3. `load()` resolves module paths against the require table and loads `.scampi` files
4. Steps from modules work identically to built-in steps

## Sections

{{< cards >}}
  {{< card link="format" title="Module Format" subtitle="scampi.mod, scampi.sum, and entry points" >}}
  {{< card link="publishing" title="Publishing" subtitle="Repository layout, versioning, and conventions" >}}
  {{< card link="testing" title="Testing" subtitle="Test modules with scampi test and in-memory targets" >}}
  {{< card link="local" title="Local Modules" subtitle="Develop modules alongside your project" >}}
{{< /cards >}}
