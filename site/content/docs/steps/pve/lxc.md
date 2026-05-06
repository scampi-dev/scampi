---
title: lxc
---

Manage an LXC container's lifecycle on a Proxmox VE host. Creates the
container from a template if it doesn't exist, ensures it matches the
desired running state on every check.

The step shells into the PVE host (passed as an `ssh.target`) and uses
`pct` for all operations. To run steps *inside* the container after it's
up, use [`pve.lxc_target`]({{< relref "../../targets/pve" >}}).

```scampi {filename="provision.scampi"}
import "std/pve"

let pve_host = ssh.target { name = "pve", host = "10.0.0.5", user = "root" }

std.deploy(name = "create-box", targets = [pve_host]) {
  pve.lxc {
    id       = 200
    node     = "pve"
    hostname = "box"
    memory   = "1G"
    networks = [pve.LxcNet { name = "eth0", bridge = "vmbr0", ip = "10.0.0.20/24" }]
  }
}
```

## Fields

### Required

| Field      | Type   | Description                      |
| ---------- | ------ | -------------------------------- |
| `id`       | int    | LXC container ID (must be ≥ 100) |
| `node`     | string | PVE cluster node name            |
| `hostname` | string | Container hostname               |

### Container resources

| Field        | Type        | Default       | Description                                    |
| ------------ | ----------- | ------------- | ---------------------------------------------- |
| `template`   | `Template?` |               | OS template to create from (storage + name)    |
| `memory`     | string      | `"512M"`      | RAM allocation (size suffix: K, M, G)          |
| `swap`       | string?     | (= `memory`)  | Swap allocation                                |
| `storage`    | string      | `"local-zfs"` | Root disk storage pool                         |
| `size`       | string      | `"8G"`        | Root disk size                                 |
| `cpu`        | `Cpu?`      |               | CPU configuration (cores, limit, weight)       |
| `privileged` | bool        | `false`       | Run as a privileged container (less isolation) |

### Networking and DNS

| Field      | Type            | Default | Description                                   |
| ---------- | --------------- | ------- | --------------------------------------------- |
| `networks` | `list\[LxcNet]` | `[]`    | Network interfaces (index → net0, net1, …)    |
| `dns`      | `Dns?`          |         | DNS overrides; defaults inherit from PVE host |

### Storage

| Field    | Type           | Default | Description                                  |
| -------- | -------------- | ------- | -------------------------------------------- |
| `mounts` | `list\[Mount]` | `[]`    | Bind mounts (`bind_mount`) and volume mounts |

### Lifecycle and behaviour

| Field             | Type            | Default            | Description                                            |
| ----------------- | --------------- | ------------------ | ------------------------------------------------------ |
| `state`           | `LxcState`      | `LxcState.running` | Desired state: `running`, `stopped`, or `absent`       |
| `startup`         | `Startup?`      |                    | Boot ordering and timing                               |
| `features`        | `Features?`     |                    | Advanced LXC features (nesting, keyctl, fuse, …)       |
| `devices`         | `list\[Device]` | `[]`               | Host device passthrough (PVE 8.1+)                     |
| `tags`            | `list\[string]` | `[]`               | Datacenter tags                                        |
| `password`        | string?         |                    | Root password (set at create time)                     |
| `ssh_public_keys` | `list\[string]` | `[]`               | SSH public keys injected into root's `authorized_keys` |
| `desc`            | string?         |                    | Human-readable description                             |
| `on_change`       | `list\[Step]`   | `[]`               | Steps to trigger when this container changes           |

## Composite types

### `LxcNet` — network interface

```scampi
pve.LxcNet {
  name     = "eth0"               // optional; defaults to ethN
  bridge   = "vmbr0"
  ip       = "10.0.0.20/24"       // CIDR or "dhcp"
  gw       = "10.0.0.1"           // optional
  vlan_tag = 0                    // 0 = no VLAN
  mac      = "BE:EF:CA:FE:00:01"  // optional; pin the hardware address
}
```

List index determines the PVE net index — first element becomes `net0`,
second `net1`, and so on.

Pinning `mac` is useful when you need to reserve the IP on an upstream
DHCP server **before** the LXC is created — otherwise PVE generates a
random MAC and the reservation can't match. Comparison is
case-insensitive; the value is normalised to uppercase to match what
PVE stores.

### `Cpu` — CPU configuration

```scampi
pve.Cpu {
  cores  = 2          // number of cores
  limit  = "1.5"      // optional max total CPU usage
  weight = 0          // optional scheduler weight
}
```

### `Dns` — DNS overrides

```scampi
pve.Dns {
  nameserver   = ["10.0.0.1", "1.1.1.1"]
  searchdomain = "lan"
}
```

`nameserver` is a list. PVE stores them as a single space-separated
line internally, but the API takes resolvers declaratively. If neither
field is set, the container inherits DNS settings from the PVE host.

### `Features` — advanced LXC features

```scampi
pve.Features {
  nesting      = true   // required for systemd inside the container
  keyctl       = false  // required for Docker in unprivileged containers
  fuse         = false
  mknod        = false  // experimental, kernel 5.3+
  force_rw_sys = false
  mount        = []     // allowed mount fs types, e.g. ["nfs", "cifs"]
}
```

Changing features on a running container triggers a reboot.

### `Startup` — boot ordering

```scampi
pve.Startup {
  on_boot = true
  order   = 1     // lower numbers start first
  up      = 30    // seconds before next start
  down    = 0     // seconds before next stop
}
```

Shutdown order is the reverse of startup order.

### `Mount` — produced by `bind_mount` or `volume_mount`

```scampi
pve.bind_mount {
  source     = "/srv/data"   // absolute path on the PVE host
  mountpoint = "/data"       // absolute path inside the container
  ro         = false
  backup     = true
}

pve.volume_mount {
  storage    = "local-zfs"
  mountpoint = "/var/lib/postgres"
  size       = "20G"
}
```

List index determines the PVE mount index — first element becomes `mp0`,
second `mp1`, and so on.

### `Template` — OS template

```scampi
pve.Template {
  storage = "local"  // template storage pool
  name    = "debian-12-standard_12.7-1_amd64.tar.zst"
}
```

## Examples

### Minimal — just create a running container

```scampi
pve.lxc {
  id       = 200
  node     = "pve"
  hostname = "box"
}
```

### Systemd container with a bind mount

```scampi
pve.lxc {
  id       = 201
  node     = "pve"
  hostname = "app"
  memory   = "2G"
  features = pve.Features { nesting = true }
  networks = [pve.LxcNet { ip = "10.0.0.21/24", gw = "10.0.0.1" }]
  mounts   = [pve.bind_mount { source = "/srv/app/data", mountpoint = "/data" }]
}
```

### Stopped (but kept) container

```scampi
pve.lxc {
  id       = 202
  node     = "pve"
  hostname = "archive"
  state    = pve.LxcState.stopped
}
```

### Removing a container

```scampi
pve.lxc {
  id       = 203
  node     = "pve"
  hostname = "old-box"
  state    = pve.LxcState.absent
}
```
