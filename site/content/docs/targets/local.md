---
title: local
---

Run steps on the machine where scampi is invoked.

```scampi
import "std/local"

let machine = local.target { name = "my-machine" }
```

## Fields

| Field  | Type   | Required | Description                            |
| ------ | ------ | :------: | -------------------------------------- |
| `name` | string |    ✓     | Identifier referenced by deploy blocks |

## How it works

The local target executes commands directly on the host. It detects the OS,
package manager, init system, container runtime, and privilege escalation tool
(sudo/doas) automatically.
