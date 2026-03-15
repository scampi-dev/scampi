---
title: firewall
---

Manage firewall rules via UFW or firewalld.

## Fields

| Field    | Type   | Required | Default   | Description                               |
|----------|--------|:--------:|-----------|-------------------------------------------|
| `port`   | string |    ✓     |           | Port/protocol string                      |
| `action` | string |          | `"allow"` | Rule action: `allow`, `deny`, or `reject` |
| `desc`   | string |          |           | Human-readable description                |

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
  `service()` step to enable UFW itself.
- **Apply**: runs `ufw <action> <port>`.

### firewalld

- **Check (allow)**: runs `firewall-cmd --query-port=<port>`.
- **Check (deny/reject)**: runs `firewall-cmd --query-rich-rule='...'`.
- **Apply (allow)**: runs `firewall-cmd --permanent --add-port=<port>` then
  `firewall-cmd --reload`.
- **Apply (deny/reject)**: adds a rich rule with `--permanent` then reloads.

The `--permanent` + `--reload` pattern ensures rules persist across reboots.

### Action mapping

| Action   | UFW command         | firewalld command       |
|----------|---------------------|-------------------------|
| `allow`  | `ufw allow 22/tcp`  | `--add-port=22/tcp`     |
| `deny`   | `ufw deny 22/tcp`   | rich rule with `drop`   |
| `reject` | `ufw reject 22/tcp` | rich rule with `reject` |

## Port format

The `port` field accepts:

| Format                  | Example         | Description |
|-------------------------|-----------------|-------------|
| `<port>/<proto>`        | `22/tcp`        | Single port |
| `<start>:<end>/<proto>` | `6000:6007/tcp` | Port range  |

Protocol must be `tcp` or `udp`.

## Examples

### Allow SSH

```python {filename="deploy.star"}
firewall(
    port="22/tcp",
    action="allow",
    desc="allow SSH",
)
```

### Allow HTTP and HTTPS

```python {filename="deploy.star"}
firewall(port="80/tcp", desc="allow HTTP")
firewall(port="443/tcp", desc="allow HTTPS")
```

### Deny a port

```python {filename="deploy.star"}
firewall(
    port="3306/tcp",
    action="deny",
    desc="block MySQL from outside",
)
```

### Server hardening pattern

```python {filename="harden.star"}
pkg(
    packages=["ufw"],
    state="present",
    desc="install firewall",
)

firewall(port="22/tcp", desc="allow SSH")
firewall(port="80/tcp", desc="allow HTTP")
firewall(port="443/tcp", desc="allow HTTPS")

service(name="ufw", state="running", enabled=True)
```
