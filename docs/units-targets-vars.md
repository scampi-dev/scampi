# Configuration Model

This document describes `scampi`'s configuration model: how targets are defined,
how deploy blocks organize work, how environment-specific values flow through
the system, and how project layout scales.

---

## Design Principles

1. **Explicit over implicit** — No magic precedence, no hidden conventions
2. **Complexity is opt-in** — Single file works; split when it hurts
3. **Target is truth** — Stateless execution, always inspect live state
4. **Batteries included** — Steps are built-in, not plugins
5. **Not everything executable deserves identity** — Steps are anonymous; units are named

---

## The Core Abstraction

Two fundamentally different concepts:

| Concept | Has identity? | Needs ordering?               | Example                        |
| ------- | ------------- | ----------------------------- | ------------------------------ |
| Unit    | Yes           | No (unordered across deploys) | nginx, postgres, base-packages |
| Step    | No            | Yes (ordered within a block)  | copy file, template config     |

**Design rules:**
- If naming feels awkward, it's not a unit — it's a step
- Units describe *what* (desired state), steps describe *how* (convergence actions)
- Identity belongs only at convergence boundaries
- Ordering belongs only to procedural sequences

> **Units are not yet implemented.** The current system works entirely with
> inline steps. The unit concept is preserved here because it will inform the
> reusable-module design.

---

## Targets

### What a Target Is

A **target** describes how to reach a machine. Nothing more.

```python
target.local(name="laptop")

target.ssh(
    name="web1",
    host="10.0.0.1",
    user="deploy",
)
```

### What a Target Is NOT

- No tags
- No metadata
- No behavioral quirks
- No variable overrides

### Target Types

| Type    | Builtin                                  | Use case                                             |
| ------- | ---------------------------------------- | ---------------------------------------------------- |
| `local` | `target.local(name)`                     | Local machine execution                              |
| `ssh`   | `target.ssh(name, host, user, ...)`      | Remote execution via SSH                             |
| `rest`  | `target.rest(name, base_url, auth, ...)` | API-driven services (planned, see `docs/roadmap.md`) |

SSH targets accept additional keyword arguments:

```python
target.ssh(
    name="web1",
    host="10.0.0.1",
    user="deploy",
    port=2222,              # optional
    key="~/.ssh/id_rsa",    # optional
    insecure=True,          # optional: skip host key verification
    timeout="10s",          # optional: connection timeout
)
```

---

## Steps

### Batteries Included

Steps are the executable primitives. They are **built into `scampi`**, not plugins.

Current steps: `copy`, `dir`, `symlink`, `template`, `pkg`, `service`, `run`

Planned steps: see `docs/roadmap.md`

### Why No Plugins

- Security boundary is clear
- No version matrix hell
- Consistent quality and documentation
- If something's missing, contribute to `scampi`, don't fork

### Step Builtins

```python
copy(src=local("./app.conf"), dest="/etc/app.conf", perm="0644", owner="root", group="root")

dir(path="/var/app", perm="0755", owner="app", group="app")

symlink(target="/etc/app.conf", link="/opt/app/config")

template(
    src=local("./nginx.conf.tmpl"),  # or src=inline("{{ .template }}")
    dest="/etc/nginx/nginx.conf",
    perm="0644",
    owner="root",
    group="root",
    data={"values": {"workers": 4, "port": 80}},
)

pkg(packages=["nginx", "curl"], state="present", source=system())  # present | latest | absent
```

Every step accepts an optional `desc` keyword for human-readable descriptions.

---

## Deploy Blocks

A **deploy block** binds targets to an ordered sequence of steps.

```python
deploy(
    name="web",
    targets=["web1", "web2"],
    steps=[
        copy(src=local("./motd.txt"), dest="/etc/motd", perm="0644", owner="root", group="root"),
        pkg(packages=["nginx"], source=system()),
        template(
            src=local("./nginx.conf.tmpl"),
            dest="/etc/nginx/nginx.conf",
            perm="0644",
            owner="root",
            group="root",
            data={"values": {"workers": 4}},
        ),
    ],
)
```

Reading order matches execution: "On web1 and web2, run: copy, pkg, template."

---

## Environment Variables

### The Ansible Trap

Ansible has 22 levels of variable precedence. "Where did this value come from?"
is often unanswerable.

### scampi Approach: `env()`

The `env()` builtin reads environment variables with optional defaults and type
coercion:

```python
# Required — errors if not set
db_host = env("DB_HOST")

# With default — returns default if not set
port = env("SSH_PORT", 2222)         # coerces to int
debug = env("DEBUG", False)          # coerces to bool
level = env("LOG_LEVEL", "info")     # string
```

Type coercion follows the default value's type:
- **int**: parses the env var as an integer
- **bool**: `"true"`, `"1"`, `"yes"` → True; `"false"`, `"0"`, `"no"`, `""` → False
- **string**: no coercion

### Provenance is Obvious

Every value is either a literal in your `.scampi` file or an explicit `env()` call.
There's no layering, no precedence, no hidden override mechanism. You can always
answer "where did this value come from?" by reading the code.

### Secrets

The `secret()` builtin resolves sensitive values at eval time from a pluggable
backend. Secrets are always required (no default value).

```python
db_password = secret("postgres.admin.password")
api_token = secret("npm.api_token")
```

V1 ships with an `unencrypted_file` backend that reads a flat JSON
`secrets.json` next to the config file. Additional backends (age, Bitwarden,
Vault, etc.) are planned — the backend is selected by the user, not by scampi.
See `docs/roadmap.md` for the full design direction.

Secret values never appear in diagnostic output — only key names.

---

## Multi-File Configs

### `import` for Splitting

Standard `import` splits configs across files:

```python
# config.scampi
module main

import "myproject/targets"
import "myproject/steps/web"

deploy(name="web", targets=targets.web_targets, steps=web.web_steps)
```

```python
# targets.scampi
module myproject/targets

import "std"

pub web_targets = ["web1", "web2"]
pub db_targets  = ["db1"]

std.target.ssh(name="web1", host="10.0.0.1", user="deploy")
std.target.ssh(name="web2", host="10.0.0.2", user="deploy")
```

Imported modules expose names via `pub`. `target.*()` and `deploy()` calls
in imported files register globally with the engine.

### Target Grouping

Groups are plain lists. Composition is just list operations:

```python
web = ["web1", "web2"]
db = ["db1"]
all = web + db
```

No special group abstraction needed — scampi is a real language.

---

## Project Layout

### Complexity is Opt-In

Start with one file. Split when it hurts.

### Minimal (Single File)

```python
# dotfiles.scampi
target.local(name="laptop")

deploy(
    name="dotfiles",
    targets=["laptop"],
    steps=[
        symlink(target="bashrc", link="~/.bashrc"),
        symlink(target="vimrc", link="~/.vimrc"),
    ],
)
```

```bash
scampi apply dotfiles.scampi
```

### Small (One File, Multiple Targets)

```python
# config.scampi
target.local(name="laptop")
target.ssh(name="server", host="myserver.com", user="me")

deploy(
    name="dotfiles",
    targets=["laptop", "server"],
    steps=[
        symlink(target="bashrc", link="~/.bashrc"),
    ],
)
```

### Medium (Split Files)

```
myproject/
├── targets.scampi
└── config.scampi
```

### Large (Full Layout)

```
myproject/
├── targets/
│   ├── test.scampi
│   ├── stage.scampi
│   └── prod.scampi
├── steps/
│   ├── web.scampi
│   └── db.scampi
└── config.scampi
```

---

## CLI Flags

| Flag                  | Meaning                                            |
| --------------------- | -------------------------------------------------- |
| `--targets <names>`   | Filter to specific target names (comma-separated)  |
| `--only <blocks>`     | Filter to specific deploy blocks (comma-separated) |
| `-v` / `-vv` / `-vvv` | Increase verbosity                                 |
| `--color <mode>`      | Colorize output: auto, always, never               |
| `--ascii`             | Force ASCII output (no Unicode glyphs)             |

### Examples

```bash
# Minimal: single file
scampi apply dotfiles.scampi

# Check without applying
scampi check config.scampi

# Show execution plan
scampi plan config.scampi

# Filter to specific targets
scampi apply --targets web1,web2 config.scampi

# Filter to specific deploy block
scampi apply --only web config.scampi

# Inspect file diffs
scampi inspect config.scampi
```

---

## Execution Model

### Stateless by Design

There is no state file. The target is truth.

Every execution:
1. **Check** — Inspect actual target state
2. **Compare** — Is it already correct?
3. **Execute** — Only if needed
4. **Verify** — Confirm result

```
Check() → "is the file already correct?"
  ↓ yes → skip (green)
  ↓ no  → Execute() → verify (yellow)
```

### Why No State File

- No team coordination nightmare (locking, remote backends)
- No "plan succeeds, apply crashes, partial state" scenarios
- No drift blindness (manual changes are visible immediately)
- Idempotent by design — run 100 times, same result

The target IS the truth. You cannot avoid inspecting it. Caching that
inspection is a liability, not an optimization.

---

## Summary

| Concept       | How                                     | Key property                    |
| ------------- | --------------------------------------- | ------------------------------- |
| Steps         | Built-in functions (`copy`, `dir`, ...) | Batteries included              |
| Targets       | `target.local()`, `target.ssh()`        | Pure identity                   |
| Deploy blocks | `deploy(name, targets, steps)`          | Host-centric: "on X run Y"      |
| Environment   | `env(key, default?)`                    | Explicit, no precedence         |
| Secrets       | `secret(key)`                           | Pluggable backend, never logged |
| Multi-file    | `import`                                | Standard scampi                 |
| Groups        | Plain lists                             | Just scampi                     |

Complexity scales from one file to full layout. Explicit over implicit.
Target is truth.
