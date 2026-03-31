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

```python {filename="deploy.scampi"}
target.local(name="my-machine")

deploy(
    name = "hello",
    targets = ["my-machine"],
    steps = [
        dir(path="/tmp/scampi-demo", perm="0755"),
        dir(path="/tmp/scampi-demo/v1", perm="0755"),
        symlink(
            target = "/tmp/scampi-demo/v1",
            link = "/tmp/scampi-demo/current",
        ),
    ],
)
```

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

To manage a remote machine over SSH, add a target:

```python {filename="remote.scampi"}
target.ssh(name="web", host="192.168.1.10", user="deploy")

deploy(
    name = "hello",
    targets = ["web"],
    steps = [
        dir(path="/tmp/scampi-demo", perm="0755"),
        copy(
            src = local("./message.txt"),
            dest = "/tmp/scampi-demo/message.txt",
            perm = "0644", owner = "root", group = "root",
        ),
    ],
)
```

Scampi connects via SSH — no agents or daemons needed on the remote host.

## Next steps

- Read [Concepts]({{< relref "concepts" >}}) to understand the execution model
- See [Configuration]({{< relref "configuration" >}}) for targets, deploy blocks, and variables
- Browse the [Step Reference]({{< relref "steps" >}}) for all built-in step types
