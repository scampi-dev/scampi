---
title: Getting Started
weight: 1
---

This guide walks you through installing scampi and writing your first
configuration.

## Install

Build from source (requires Go 1.25+):

```bash
git clone https://codeberg.org/pskry/scampi.git
cd scampi
just build
```

The binary lands at `./build/bin/scampi`.

## Your first config

{{% steps %}}

### Create a config file

```python {filename="deploy.star"}
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
scampi check deploy.star
```

Scampi inspects the current state and reports what differs from your declared
config. Yellow means something would change. Green means it already matches.

### Apply

```bash
scampi apply deploy.star
```

Run it again — everything should be green. That's convergence: the system already
matches your declared state, so there's nothing to do.

{{% /steps %}}

## Add a remote target

To manage a remote machine over SSH, add a target:

```python {filename="remote.star"}
target.ssh(name="web", host="192.168.1.10", user="deploy")

deploy(
    name = "hello",
    targets = ["web"],
    steps = [
        dir(path="/tmp/scampi-demo", perm="0755"),
        copy(
            src = "./message.txt",
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
