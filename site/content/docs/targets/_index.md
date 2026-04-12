---
title: Target Reference
weight: 5
---

A target defines where steps execute. Each target type provides a different
transport — local shell, SSH, or HTTP — but they all plug into the same deploy
block mechanism.

```scampi
import "std"
import "std/ssh"

let web = ssh.target { name = "web", host = "app.example.com", user = "deploy" }

std.deploy(name = "webserver", targets = [web]) {
  // ... steps
}
```

Targets are declared with `let` bindings and passed into `std.deploy(...)` by
reference. A single config can declare multiple targets of different types and
bind them to different deploy blocks.

Each target type lives in its own module under `std/`:

| Module      | Target         | Use for                                    |
| ----------- | -------------- | ------------------------------------------ |
| `std/local` | `local.target` | Steps that run on the machine scampi is on |
| `std/ssh`   | `ssh.target`   | Steps that run on a remote host over SSH   |
| `std/rest`  | `rest.target`  | HTTP requests against a REST API           |

## Available targets

{{< cards >}}
  {{< card link="local" title="local" subtitle="Run steps on the local machine" >}}
  {{< card link="ssh" title="ssh" subtitle="Run steps on a remote host via SSH" >}}
  {{< card link="rest" title="rest" subtitle="Make HTTP requests against a REST API" >}}
{{< /cards >}}
