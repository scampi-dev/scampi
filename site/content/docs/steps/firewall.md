---
title: firewall
---

Manage firewall rules via UFW or firewalld.

## Fields

| Field       | Type             | Required | Default                | Description                                      |
| ----------- | ---------------- | :------: | ---------------------- | ------------------------------------------------ |
| `port`      | string           |    ✓     |                        | Port/protocol string — see [below](#port-format) |
| `action`    | `FirewallAction` |          | `FirewallAction.allow` | Rule action                                      |
| `desc`      | string?          |          |                        | Human-readable description                       |
| `on_change` | list\[Step]      |          |                        | Steps to trigger when this rule changes          |

## Actions

`posix.FirewallAction` is an enum:

| Value                   | UFW command         | firewalld command       |
| ----------------------- | ------------------- | ----------------------- |
| `FirewallAction.allow`  | `ufw allow 22/tcp`  | `--add-port=22/tcp`     |
| `FirewallAction.deny`   | `ufw deny 22/tcp`   | rich rule with `drop`   |
| `FirewallAction.reject` | `ufw reject 22/tcp` | rich rule with `reject` |

## How it works

The step manages a single firewall rule per call. It auto-detects the firewall
backend on the target and dispatches to the appropriate tool.

### Backend detection

On every check, scampi probes for a supported backend:

1. `ufw version` — if exit 0, use UFW
2. `firewall-cmd --version` — if exit 0, use firewalld
3. Neither found → error with hint to install one

### UFW

- **Check**: runs `ufw show added` and looks for `ufw <action> <port>`. This
  works even when UFW is inactive — rules are stored, just not enforced. Use the
  `service` step to enable UFW itself.
- **Apply**: runs `ufw <action> <port>`.

### firewalld

- **Check (allow)**: runs `firewall-cmd --query-port=<port>`.
- **Check (deny/reject)**: runs `firewall-cmd --query-rich-rule='...'`.
- **Apply (allow)**: runs `firewall-cmd --permanent --add-port=<port>` then
  `firewall-cmd --reload`.
- **Apply (deny/reject)**: adds a rich rule with `--permanent` then reloads.

The `--permanent` + `--reload` pattern ensures rules persist across reboots.

## Port format

The `port` field is validated against the regex
`^[0-9]+(-[0-9]+)?(/(tcp|udp))?$` and accepts:

| Format                  | Example         | Description |
| ----------------------- | --------------- | ----------- |
| `<port>/<proto>`        | `22/tcp`        | Single port |
| `<start>-<end>/<proto>` | `6000-6007/tcp` | Port range  |

Protocol must be `tcp` or `udp`.

## Examples

### Allow SSH

```scampi {filename="deploy.scampi"}
posix.firewall {
  port = "22/tcp"
  desc = "allow SSH"
}
```

### Allow HTTP and HTTPS

```scampi {filename="deploy.scampi"}
posix.firewall { port = "80/tcp", desc = "allow HTTP" }
posix.firewall { port = "443/tcp", desc = "allow HTTPS" }
```

### Deny a port

```scampi {filename="deploy.scampi"}
posix.firewall {
  port   = "3306/tcp"
  action = posix.FirewallAction.deny
  desc   = "block MySQL from outside"
}
```

### Server hardening pattern

```scampi {filename="harden.scampi"}
posix.pkg {
  packages = ["ufw"]
  state    = posix.PkgState.present
  source   = posix.pkg_system {}
  desc     = "install firewall"
}

posix.firewall { port = "22/tcp", desc = "allow SSH" }
posix.firewall { port = "80/tcp", desc = "allow HTTP" }
posix.firewall { port = "443/tcp", desc = "allow HTTPS" }

posix.service { name = "ufw", state = posix.ServiceState.running, enabled = true }
```
