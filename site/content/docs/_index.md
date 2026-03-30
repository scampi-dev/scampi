---
title: Documentation
layout: docs
sidebar:
  open: true
---

Welcome to the scampi documentation.

Scampi is a declarative system convergence engine. You describe the desired state
of your systems in [Starlark](https://github.com/google/starlark-go) — a
deterministic, Python-like configuration language — and scampi executes
idempotent operations to make reality match.

## Where to start

{{< cards >}}
  {{< card link="getting-started" title="Getting Started" subtitle="Install scampi and write your first config" >}}
  {{< card link="concepts" title="Concepts" subtitle="Understand the mental model: steps, actions, ops, targets" >}}
  {{< card link="configuration" title="Configuration" subtitle="Deploy blocks, source resolvers, secrets, and project layout" >}}
  {{< card link="modules" title="Modules" subtitle="Reusable Starlark packages, local development, and testing" >}}
  {{< card link="targets" title="Target Reference" subtitle="Local, SSH, and REST target types" >}}
  {{< card link="steps" title="Step Reference" subtitle="Every built-in step type, with fields and examples" >}}
  {{< card link="cli" title="CLI" subtitle="Subcommands, flags, output semantics, and exit codes" >}}
  {{< card link="philosophy" title="Philosophy" subtitle="Why convergence, why Starlark, why opinions over options" >}}
{{< /cards >}}
