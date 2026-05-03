---
title: ssh
---

Run steps on a remote host via SSH.

```scampi
import "std/ssh"

let web = ssh.target {
  name = "web"
  host = "app.example.com"
  user = "deploy"
}
```

## Fields

| Field          | Type    | Required | Default | Description                                   |
| -------------- | ------- | :------: | ------- | --------------------------------------------- |
| `name`         | string  |    ✓     |         | Identifier for deploy blocks                  |
| `host`         | string  |    ✓     |         | Hostname or IP address                        |
| `user`         | string  |    ✓     |         | SSH user                                      |
| `port`         | int     |          | `22`    | SSH port                                      |
| `key`          | string? |          |         | Path to private key file                      |
| `insecure`     | bool?   |          |         | Skip host key verification                    |
| `timeout`      | string  |          | `"5s"`  | Connection timeout (Go format)                |
| `max_sessions` | int     |          | `10`    | Max concurrent SSH sessions (see Concurrency) |

## Authentication

SSH targets try authentication methods in order:

1. **Explicit key** — if `key` is set, the private key file is loaded and used.
2. **SSH agent** — if `$SSH_AUTH_SOCK` is set, the agent is queried for keys.

At least one method must succeed. If neither is available, scampi reports an
error with guidance on how to configure authentication.

## Host key verification

By default, scampi verifies host keys against `~/.ssh/known_hosts`. Set
`insecure = true` to skip verification — useful for ephemeral test
environments, but not recommended for production.

## How it works

On connection, the SSH target probes the remote system to detect the OS,
package manager, init system, container runtime, and privilege escalation tool
(sudo/doas). This determines which step capabilities are available.

## Concurrency

scampi opens a single SSH connection per target and reuses it for every
operation. Each individual command opens an SSH session (channel) on that
shared connection — sessions are cheap (no TCP, no auth, no key exchange),
but the SSH server caps how many can be open at once.

Two safeguards keep that under control:

1. **Client-side sanity cap** — `max_sessions` (default `10`, matching
   OpenSSH's `sshd_config` default) bounds how many concurrent sessions
   scampi will open against this target. Excess parallel ops queue inside
   scampi instead of being fired at the server. Raise this if you've lifted
   `MaxSessions` server-side and want scampi to use the headroom.
2. **Server backpressure** — if the server still rejects a session open
   (because its actual limit is lower, or transient load), scampi retries
   with exponential backoff and jitter. The server is the source of truth
   on what it'll accept; scampi listens and adapts.

Connect-time failures (`MaxStartups` rejection, brief network blips) are
also retried with backoff. Auth errors and unresolvable hosts bail
immediately.
