---
title: check
weight: 2
---

```text
scampi check [flags] <config>
```

Reads a declarative configuration file and inspects the target system to
determine which operations are already satisfied and which would need to execute.

No changes are made to the system. Unlike [plan](../plan), this command
evaluates the actual system state.

## Flags

| Flag        | Description                                       |
|-------------|---------------------------------------------------|
| `--only`    | Filter to specific deploy blocks (comma-separated) |
| `--targets` | Filter to specific targets (comma-separated)       |
