---
title: run
---

Run an arbitrary shell command on the target. This is the escape hatch — use it
when no built-in step type fits your needs.

## Fields

| Field       | Type                | Required | Default | Description                                        |
| ----------- | ------------------- | :------: | ------- | -------------------------------------------------- |
| `apply`     | string              |    ✓     |         | Shell command to execute (`@std.nonempty`)         |
| `check`     | string?             |          |         | Shell command that exits 0 if apply is unnecessary |
| `always`    | bool?               |          | `false` | Always run apply, skip check                       |
| `env`       | map\[string,string] |          | `{}`    | Env vars passed to apply and check (shell-quoted)  |
| `desc`      | string?             |          |         | Human-readable description                         |
| `on_change` | list\[Step]         |          |         | Steps to trigger when apply runs                   |
| `promises`  | list\[string]       |          | `[]`    | Cross-deploy resources this step produces          |
| `inputs`    | list\[string]       |          | `[]`    | Cross-deploy resources this step consumes          |

Use `env` for any value with whitespace, quotes, or shell metacharacters
— scampi shell-quotes each value automatically. Combine with
`std.secret_env` to pass secrets without leaking into logs.

Provide either `check` or `always` (or neither — but at least one of them is recommended for idempotency).

## How it works

With `check`: scampi runs the check command first. If it exits 0, the desired
state is already met and `apply` is skipped. Any non-zero exit means `apply`
runs.

With `always`: the apply command runs on every invocation. This gives up
idempotency for that step — use it only for side-effect commands where checking
isn't practical.

Commands run under `/bin/sh -c` with the target's environment.

## Guarantees

| Mode               | Idempotent | Dry-run accurate | Convergence reported |
| ------------------ | ---------- | ---------------- | -------------------- |
| `check` + `apply`  | yes        | yes              | yes                  |
| `always` + `apply` | no         | no               | no                   |
| Built-in steps     | yes        | yes              | yes                  |

The `always` mode is intentionally degraded — scampi can't know what your
command does, so it can't make promises about it.

## Examples

### With check

```scampi
posix.run {
  desc  = "enable IP forwarding"
  check = "sysctl net.ipv4.ip_forward | grep -q '= 1'"
  apply = "sysctl -w net.ipv4.ip_forward=1"
}
```

### Extract an archive

```scampi
posix.run {
  desc  = "extract site content"
  check = "test /var/www/site/index.html -nt /tmp/site.tar.gz"
  apply = "tar xzf /tmp/site.tar.gz -C /var/www/site"
}
```

### Always run

```scampi
posix.run {
  desc   = "reload nginx config"
  always = true
  apply  = "nginx -s reload"
}
```

### Migration on-ramp

You can wrap existing tools as a stepping stone:

```scampi
posix.run {
  desc   = "run ansible playbook"
  always = true
  apply  = "ansible-playbook -i inventory site.yml"
}
```

This is deliberately ugly — it works, but you lose all convergence guarantees.
The intent is to give you a migration path, not a permanent escape.
