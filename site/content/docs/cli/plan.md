---
title: plan
weight: 3
---

```text
scampi plan [flags] <config>
```

Reads a declarative configuration file and prints the execution plan without
applying any changes.

The plan shows the operations that would be executed by [apply](../apply), but
does not inspect or modify the target system. Unlike [check](../check), it
shows intent only — it doesn't evaluate whether the system already satisfies
the desired state.

## Flags

| Flag        | Description                                        |
|-------------|----------------------------------------------------|
| `--only`    | Filter to specific deploy blocks (comma-separated) |
| `--targets` | Filter to specific targets (comma-separated)       |
