# The Escape Hatch

## Problem

The declarative model works when a step type exists for the thing you need. But
there's always stuff that doesn't fit: a one-off sysctl tweak, a custom build
script, a config transformation that no step type covers yet. Without an escape
hatch, users either can't use scampi for their full setup, or they wrap it in a
shell script — which defeats the purpose.

Every config management tool that tried to be purely declarative eventually
added an escape hatch: Puppet's `exec`, Ansible's `command`/`shell`, NixOS's
`system.activationScripts`.

## What a naive escape hatch breaks

A bare `run: "do the thing"` step blows up the engine's guarantees:

- **Idempotency** — the engine can't know if the command is safe to re-run.
- **Convergence reporting** — no way to distinguish "already correct" from
  "changed" because the engine doesn't know what "correct" looks like.
- **Dry-run** — you can't preview what an opaque shell command will do.
- **Fail-fast** — if the command is non-deterministic, retry semantics are
  unclear.

## Design: check/execute pair

The escape hatch should preserve the engine's check/execute model by requiring
the user to provide both halves of the contract:

```python
run(
    name = "enable ip forwarding",
    check = "sysctl net.ipv4.ip_forward | grep -q '= 1'",
    apply = "sysctl -w net.ipv4.ip_forward=1",
)
```

**check** (exit 0 = already correct): the engine runs this first. If it
succeeds, the step is skipped and reported as "ok." If it fails, the engine
runs **apply**, then re-runs **check** to verify convergence.

This gives us:
- Real convergence reporting ("already correct" vs "changed")
- Dry-run works (run check only, report what would change)
- Idempotency burden is explicit — the user carries it, not the engine
- Fail-fast works (non-zero exit from apply or post-apply check = failure)

## Degraded mode: always-run

If the user genuinely can't write a check (side-effect-only commands, scripts
with no observable state), they should be able to opt in explicitly:

```python
run(
    name = "refresh something",
    apply = "do-the-thing",
    always = True,
)
```

This always runs, always reports as "changed." The engine doesn't pretend to
know anything. This is an explicit contract degradation, not the default.

## Reporting

The CLI output should make escape-hatch steps visually distinct from
fully-declarative steps. A different glyph, a dim annotation — something that
makes it obvious at a glance which parts of your config are verified-convergent
and which are "trust me" territory.

Steps with `always = True` should be even more visually flagged. The goal is a
gentle nudge: if you see a lot of always-run steps, maybe some of them deserve
a proper step type.

## Guarantees summary

| Mode                | Idempotent   | Dry-run | Convergence report |
| ------------------- | ------------ | ------- | ------------------ |
| check + apply       | Yes (user)   | Yes     | Yes                |
| always + apply      | No           | No      | Always "changed"   |
| Built-in step types | Yes (engine) | Yes     | Yes                |

## Migration on-ramp

The escape hatch isn't just an emergency exit — it's the adoption path. Asking
users to rewrite their entire setup before they can use scampi is a non-starter.
The `run` step lets people start with what they have:

```python
run(
    name = "legacy ansible setup",
    check = "ansible-playbook site.yml --check | grep -q 'changed=0'",
    apply = "ansible-playbook site.yml",
)
```

Or wrapping Terraform:

```python
run(
    name = "infrastructure",
    check = "terraform plan -detailed-exitcode",  # exit 2 = changes needed
    apply = "terraform apply -auto-approve",
)
```

Users start here, then peel off steps one at a time. Each conversion from `run`
to a native step is visible in the convergence report: more verified-convergent
checkmarks, fewer "trust me" glyphs. The report itself becomes the migration
progress indicator.

This also means dedicated interop steps (e.g. `ansible(playbook = "site.yml")`)
are just sugar on top of `run` with pre-built check/apply pairs — they fall out
of the escape hatch design for free when the time comes.

## Shell and environment

**Shell**: POSIX `sh` — same as every other command scampi runs today
(`exec.CommandContext(ctx, "sh", "-c", cmd)` locally, bare string over SSH).
If someone needs bash features, they write `bash -c '...'` in their command.
No `shell` parameter in v1.

**Environment**: inherited from the target. Local = scampi's process env.
SSH = remote session env. Users set vars inline (`FOO=bar do-thing`).
No `env` dict in v1 — inline works fine and keeps the step type minimal.

## Escalation

No `root` flag. The user writes their command with `sudo`/`doas` if they need
it — same as if they typed it in a terminal. The escape hatch is "you're on
your own" territory and adding escalation knobs is portability theater for a
step type that runs raw shell.

What we *can* do is smart diagnostics: at plan time, inspect the command string
and cross-reference against the target's detected state. Examples:

- "your apply command uses `sudo` but this target has no sudo installed"
- "your check command uses `doas` but no escalation path is available and
  you're not root"

This is purely observational (diagnostic layer, not control flow) and catches
real mistakes without adding configuration surface. Fits the existing
diagnostic model — warn about probable problems, don't gate execution on them.

## Execution ordering

A `run` step is an opaque shell command — the engine has zero visibility into
what it reads or writes. It can't infer dependencies on files, packages,
services, or anything else. This means:

- The engine must NOT look at a `run` step's empty dependency set and conclude
  "this is independent, safe to parallelize with everything else."
- A `run` command could touch anything. The only safe behavior is to treat it
  as a **barrier**: drain all preceding steps, execute the `run` step alone,
  then continue with subsequent steps. Same idea as a memory fence in assembly
  — the compiler can't see through the clobber, so it must not reorder across
  it.

Today the engine executes actions sequentially, so the barrier falls out for
free. But this is a design constraint, not an implementation detail — if the
engine ever parallelizes independent actions (via `Pather` dependency
inference), `run` must remain a full serialization point.

This is also another reason to convert `run` steps to native step types over
time — native steps declare their dependencies, which lets the engine
parallelize them. Each `run` step is a barrier that serializes the whole plan.

## Open questions

- Should `check` support more than exit-code? E.g. comparing stdout to an
  expected value, or checking file contents. Probably not in v1 — exit-code is
  the universal contract and anything fancier can be expressed in the check
  script itself.
- Should the post-apply re-check be mandatory or opt-in? Mandatory is safer
  (proves convergence) but slower. Could default to on and allow `verify: false`
  for known-safe cases.
- Naming: `run`, `exec`, `shell`, `command`? `run` is short and doesn't collide
  with existing step names. `exec` implies replacing the process. `shell` is
  Ansible's baggage.
- Should `apply` support a list of commands (sequential execution) or force a
  single string? Single string is simpler and the user can use `&&` or a script.
  A list adds implicit sequencing semantics the engine would need to reason
  about.
