---
title: run_set
---

Reconcile a CLI-managed set against a desired identifier list. The shell-side
companion to `rest.resource_set`: you give it a `list` command (which prints
one identifier per line), an `add` command, a `remove` command, and the
desired set. scampi diffs live against desired and runs the right batch.

Use it when a CLI tool exposes set semantics — group memberships, sudoers
entries, NFS exports, allowlists — instead of vendoring a custom diff/add/remove
shell helper.

## Fields

| Field       | Type                | Required | Default | Description                                        |
| ----------- | ------------------- | :------: | ------- | -------------------------------------------------- |
| `list`      | string              |    ✓     |         | Shell command listing identifiers, one per line    |
| `add`       | string?             |          |         | Command to add items; uses an item placeholder     |
| `remove`    | string?             |          |         | Command to remove items; same placeholder shape    |
| `desired`   | list\[string]       |          | `[]`    | Identifiers that should be present                 |
| `init`      | string?             |          |         | Bootstrap command run if `list` exits non-zero     |
| `env`       | map\[string,string] |          | `{}`    | Env vars passed to every invocation (shell-quoted) |
| `desc`      | string?             |          |         | Human-readable description                         |
| `on_change` | list\[Step]         |          |         | Steps to trigger when add or remove runs           |
| `promises`  | list\[string]       |          | `[]`    | Cross-deploy resources this step produces          |
| `inputs`    | list\[string]       |          | `[]`    | Cross-deploy resources this step consumes          |

At least one of `add` or `remove` must be set. If only `add` is set, scampi
adds missing items but ignores orphans. If only `remove` is set, scampi removes
orphans but does not add. Both = full reconciliation.

## Templating

`add` and `remove` use one of three placeholders:

| Placeholder       | Behavior                                 |
| ----------------- | ---------------------------------------- |
| `{{ item }}`      | Run the command once per item            |
| `{{ items }}`     | Run once with all items, space-separated |
| `{{ items_csv }}` | Run once with all items, comma-separated |

Pick whichever matches the underlying tool's argument shape. `samba-tool group
addmembers` takes a comma-separated list, so use `{{ items_csv }}`. A loop-style
adder takes one argument per call, so use `{{ item }}`. Mixing per-item with
batch placeholders is a plan-time error.

## How it works

1. Run `list`. Parse stdout into a set of identifiers (newline-split, trimmed,
   deduped).
2. If `list` exited non-zero and `init` is set, run `init`, then re-run `list`.
3. Diff live against `desired`:
   - `desired - live` → render `add` template, run.
   - `live - desired` → render `remove` template, run (sorted for stable order).
4. `remove` runs before `add`. If both are no-ops the step is satisfied and
   nothing changes.

Commands run through the target's `RunCommand` — `/bin/sh -c` on POSIX targets,
`pct exec` on PVE LXC targets. `env` values are shell-quoted automatically.

## Examples

### Samba group membership

```scampi
let admins = ["alice", "bob", "carol"]

posix.run_set {
  desc    = "samba admins"
  list    = "samba-tool group listmembers admins"
  add     = "samba-tool group addmembers admins {{ items_csv }}"
  remove  = "samba-tool group removemembers admins {{ items_csv }}"
  desired = admins
}
```

### One-way adoption (no orphan removal)

```scampi
posix.run_set {
  desc    = "ensure baseline sudoers entries"
  list    = "ls /etc/sudoers.d"
  add     = "cp baseline/{{ item }} /etc/sudoers.d/"
  desired = ["10-ops", "20-deploy"]
}
```

Local sudoers files outside the desired list are left alone — only `add` is
declared.

### Bootstrap with `init`

```scampi
posix.run_set {
  desc    = "pi-hole adlists"
  list    = "pihole -a adlist | awk '{print $1}'"
  add     = "pihole -a adlist add {{ item }}"
  remove  = "pihole -a adlist remove {{ item }}"
  init    = "apt-get install -y pihole"
  desired = ["https://example.com/list.txt"]
}
```

`init` only runs when `list` fails (e.g. the binary is missing or the container
hasn't been provisioned yet). It's a one-shot bootstrap, not a fallback path.

## Idempotency

`run_set` is idempotent in the same sense as `rest.resource_set`: if live
equals desired, no shell commands run beyond the read-only `list`. The diff is
computed once during the check phase and consumed during apply, so commands are
deterministic and reproducible.
