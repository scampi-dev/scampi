---
title: apply
weight: 1
---

```text
scampi apply [flags] <config>
```

Reads a declarative configuration file and executes the required operations to
converge the system to the desired state.

The command is idempotent: running it multiple times only applies changes when
the current state differs from the declared state.

## Flags

| Flag        | Description                                        |
| ----------- | -------------------------------------------------- |
| `--only`    | Filter to specific deploy blocks (comma-separated) |
| `--targets` | Filter to specific targets (comma-separated)       |
