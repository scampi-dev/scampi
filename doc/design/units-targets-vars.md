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

```scampi
import "std/local"
import "std/ssh"

let laptop = local.target { name = "laptop" }

let web1 = ssh.target {
  name = "web1"
  host = "10.0.0.1"
  user = "deploy"
}
```

### What a Target Is NOT

- No tags
- No metadata
- No behavioral quirks
- No variable overrides

### Target Types

| Type    | Builtin           | Use case                 |
| ------- | ----------------- | ------------------------ |
| `local` | `local.target {}` | Local machine execution  |
| `ssh`   | `ssh.target {}`   | Remote execution via SSH |
| `rest`  | `rest.target {}`  | API-driven services      |

SSH targets accept additional fields:

```scampi
ssh.target {
  name     = "web1"
  host     = "10.0.0.1"
  user     = "deploy"
  port     = 2222              // optional, default 22
  key      = "~/.ssh/id_rsa"   // optional
  insecure = true              // optional: skip host key verification
  timeout  = "10s"             // optional, default "5s"
}
```

---

## Steps

### Batteries Included

Steps are the executable primitives. They are **built into `scampi`**, not plugins.

Current steps: `copy`, `dir`, `symlink`, `template`, `pkg`, `service`, `run`

Planned steps: see `scampi index` for the current list

### Why No Plugins

- Security boundary is clear
- No version matrix hell
- Consistent quality and documentation
- If something's missing, contribute to `scampi`, don't fork

### Step Builtins

```scampi
posix.copy {
  src   = posix.source_local { path = "./app.conf" }
  dest  = "/etc/app.conf"
  owner = "root"
  group = "root"
  perm  = "0644"
}

posix.dir {
  path  = "/var/app"
  owner = "app"
  group = "app"
  perm  = "0755"
}

posix.symlink {
  target = "/etc/app.conf"
  link   = "/opt/app/config"
}

posix.template {
  src   = posix.source_local { path = "./nginx.conf.tmpl" }
  // or: src = posix.source_inline { content = "{{ .template }}" }
  dest  = "/etc/nginx/nginx.conf"
  owner = "root"
  group = "root"
  perm  = "0644"
  data  = { "values": { "workers": 4, "port": 80 } }
}

posix.pkg {
  packages = ["nginx", "curl"]
  source   = posix.pkg_system()
  state    = posix.PkgState.present  // present | absent | latest
}
```

Every step accepts an optional `desc` field for human-readable descriptions.

---

## Deploy Blocks

A **deploy block** binds targets to an ordered sequence of steps. Steps
appear as bare expressions inside the deploy body.

```scampi
std.deploy(name = "web", targets = [web1, web2]) {
  posix.copy {
    src   = posix.source_local { path = "./motd.txt" }
    dest  = "/etc/motd"
    owner = "root"
    group = "root"
    perm  = "0644"
  }

  posix.pkg {
    packages = ["nginx"]
    source   = posix.pkg_system()
  }

  posix.template {
    src   = posix.source_local { path = "./nginx.conf.tmpl" }
    dest  = "/etc/nginx/nginx.conf"
    owner = "root"
    group = "root"
    perm  = "0644"
    data  = { "values": { "workers": 4 } }
  }
}
```

Reading order matches execution: "On web1 and web2, run: copy, pkg, template."

---

## Environment Variables

### The Ansible Trap

Ansible has 22 levels of variable precedence. "Where did this value come from?"
is often unanswerable.

### scampi Approach: `std.env()`

The `std.env()` builtin reads environment variables with an optional default:

```scampi
import "std"

// Required — errors if not set
let db_host = std.env("DB_HOST")

// With default — returns default if the env var is not set
let log_level = std.env("LOG_LEVEL", default = "info")
```

`std.env` returns a string. If you need an integer or boolean, parse the
result yourself — keeping coercion in the config file rather than the
builtin makes the type explicit at the call site.

### Provenance is Obvious

Every value is either a literal in your `.scampi` file or an explicit
`std.env()` call. There's no layering, no precedence, no hidden override
mechanism. You can always answer "where did this value come from?" by
reading the code.

### Secrets

Secret resolvers are values. Create one with `secrets.from_age` or
`secrets.from_file`, then look up keys via UFCS — `resolver.get("key")`.

```scampi
import "std/secrets"

let store = secrets.from_age(path = "secrets.age.json")

let db_password = store.get("postgres.admin.password")
let api_token   = store.get("npm.api_token")
```

Built-in resolvers:

- `secrets.from_age(path)` — age-encrypted JSON. Identity resolved from
  `$SCAMPI_AGE_KEY`, `$SCAMPI_AGE_KEY_FILE`, or `~/.config/scampi/age.key`.
- `secrets.from_file(path)` — unencrypted JSON. For development.

The linker validates literal keys against the resolver's backend at link
time. Secret values never appear in diagnostic output — only key names.

---

## Multi-File Configs

### `import` for Splitting

Standard `import` splits configs across files:

```scampi
// config.scampi
module main

import "std"
import "myproject/targets"

std.deploy(name = "web", targets = targets.web_hosts) {
  // ... steps ...
}
```

```scampi
// targets.scampi
module myproject/targets

import "std/ssh"

pub let web1 = ssh.target { name = "web1", host = "10.0.0.1", user = "deploy" }
pub let web2 = ssh.target { name = "web2", host = "10.0.0.2", user = "deploy" }

pub let web_hosts = [web1, web2]
```

Imported modules expose names via `pub`. Targets bound to `pub let`
identifiers can be referenced from any importing module.

### Target Grouping

Groups are plain lists. Composition is just list operations:

```scampi
let web = [web1, web2]
let db  = [db1]
let all = web + db
```

No special group abstraction needed — scampi is a real language.

---

## Project Layout

### Complexity is Opt-In

Start with one file. Split when it hurts.

### Minimal (Single File)

```scampi
// dotfiles.scampi
module main

import "std"
import "std/posix"
import "std/local"

let laptop = local.target { name = "laptop" }

std.deploy(name = "dotfiles", targets = [laptop]) {
  posix.symlink { target = "bashrc", link = "~/.bashrc" }
  posix.symlink { target = "vimrc",  link = "~/.vimrc" }
}
```

```bash
scampi apply dotfiles.scampi
```

### Small (One File, Multiple Targets)

```scampi
// config.scampi
module main

import "std"
import "std/posix"
import "std/local"
import "std/ssh"

let laptop = local.target { name = "laptop" }
let server = ssh.target { name = "server", host = "myserver.com", user = "me" }

std.deploy(name = "dotfiles", targets = [laptop, server]) {
  posix.symlink { target = "bashrc", link = "~/.bashrc" }
}
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

| Concept       | How                                            | Key property                    |
| ------------- | ---------------------------------------------- | ------------------------------- |
| Steps         | `posix.copy { ... }`, `posix.dir { ... }`, ... | Batteries included              |
| Targets       | `local.target { ... }`, `ssh.target { ... }`   | Pure identity                   |
| Deploy blocks | `std.deploy(name, targets) { ... }`            | Host-centric: "on X run Y"      |
| Environment   | `std.env(key, default?)`                       | Explicit, no precedence         |
| Secrets       | `secrets.from_age(...).get(key)`               | Pluggable backend, never logged |
| Multi-file    | `import`                                       | Standard scampi                 |
| Groups        | Plain lists                                    | Just scampi                     |

Complexity scales from one file to full layout. Explicit over implicit.
Target is truth.
