# Units, Targets, and Vars Design

This document captures the design for `doit`'s configuration model: how reusable units are packaged, how targets (machines/environments) are organized, and how environment-specific variables flow through the system.

---

## Design Principles

1. **Explicit over implicit** — No magic precedence, no hidden conventions
2. **Complexity is opt-in** — Single file works; full layout available when needed
3. **Target is truth** — Stateless execution, always inspect live state
4. **Developer experience is paramount** — LSP support, autocomplete, clear errors
5. **Not everything executable deserves identity** — Steps are anonymous; units are named

---

## The Core Abstraction

There are two fundamentally different concepts:

| Concept | Has identity? | Needs ordering? | Example |
|---------|---------------|-----------------|---------|
| Unit | Yes | No (unordered across deploys) | nginx, postgres, base-packages |
| Step | No | Yes (ordered within a block) | copy file, template config |

**Design rules:**
- If naming feels awkward, it's not a unit — it's a step
- Units describe *what* (desired state), steps describe *how* (convergence actions)
- Identity belongs only at convergence boundaries
- Ordering belongs only to procedural sequences

---

## The Three Layers

```
┌─────────────────────────────────────────┐
│  UNITS (what)                           │
│  Reusable convergence modules           │
│  Pure CUE, explicit params schema       │
└─────────────────────────────────────────┘
              ↓ run on
┌─────────────────────────────────────────┐
│  TARGETS (where)                        │
│  Pure identity — how to reach machines  │
│  No tags, no metadata, no quirks        │
└─────────────────────────────────────────┘
              ↓ parameterized by
┌─────────────────────────────────────────┐
│  VARS (context)                         │
│  Environment-specific values            │
│  Explicit CUE imports, no precedence    │
└─────────────────────────────────────────┘
```

---

## Units

> **Status: Design — not yet implemented**

### What a Unit Is

A **unit** is a reusable convergence module. It defines a sequence of steps that bring a system to a desired state.

Examples: `nginx`, `postgres`, `docker`, `base-packages`

### Unit Structure

```cue
// units/nginx/unit.cue
package nginx

import "godoit.dev/doit/builtin"

#Params: {
  port:      int | *80
  workers:   int | *4
  ssl_cert?: string  // optional
  ssl_key?:  string
}

#Unit: {
  params: #Params

  steps: [
    builtin.package & {name: "nginx"},
    builtin.template & {
      src:  "templates/nginx.conf.tmpl"
      dest: "/etc/nginx/nginx.conf"
      data: params
    },
    builtin.service & {name: "nginx", state: "running"},
  ]
}
```

### Key Properties

| Property | Decision |
|----------|----------|
| Content | Pure CUE + files (templates, etc.) |
| Parameters | Explicit `#Params` schema (not implicit unification) |
| Template resolution | Relative paths resolved from vendored source at runtime |
| Versioning | Go-style: `@v1.2.3`, lockfile for reproducibility |

### Packaging

Units are CUE modules, managed via `doit mod`:

```bash
doit mod init                    # create cue.mod
doit mod tidy                    # resolve dependencies
doit mod vendor                  # vendor for offline/reproducibility
```

```cue
// Importing a third-party unit
import "github.com/acme/doit-units/nginx"

// Using it
nginx.#Unit & {params: {port: 443}}
```

### Why Explicit `#Params`

- LSP can autocomplete fields and show types
- Clear contract between unit author and consumer
- Validation happens at CUE evaluation time, not runtime

---

## Steps

### Batteries Included

Steps are the executable primitives. They are **built into `doit`**, not plugins.

Current steps: `copy`, `symlink`, `template`

Planned steps: `package`, `service`, `command`, `directory`, `absent`, `user`, `group`

### Why No Plugins

- Security boundary is clear (import capability rules)
- No version matrix hell
- Consistent quality and documentation
- If something's missing, contribute to `doit`, don't fork

### Steps vs Units

| Concept | Identity | Reusable | Implemented in |
|---------|----------|----------|----------------|
| Step | Anonymous | No (inline) | Go (built-in) |
| Unit | Named | Yes (imported) | CUE (pure config) |

Units are composed of steps. Steps are the atoms.

---

## Targets

### What a Target Is

A **target** describes how to reach a machine. Nothing more.

```cue
targets: {
  web1: {type: "ssh", host: "10.0.0.1", user: "deploy"}
  web2: {type: "ssh", host: "10.0.0.2", user: "deploy"}
  local: {type: "local"}
  api: {type: "rest", endpoint: "https://config.internal/v1"}
}
```

### What a Target Is NOT

- No tags
- No metadata
- No behavioral quirks
- No variable overrides

### Target Types

| Type | Use case |
|------|----------|
| `local` | Local machine execution |
| `ssh` | Remote execution via SSH |
| `rest` | Configuration via REST API (planned) |

The `type` field determines the target interface. SSH is not the default assumption.

### Inventory Files

One file per environment:

```
inventory/
├── test.cue
├── stage.cue
├── prod.cue
└── prod-west.cue
```

```cue
// inventory/prod.cue
package inventory

targets: {
  web1: {type: "ssh", host: "10.0.0.1", user: "deploy"}
  web2: {type: "ssh", host: "10.0.0.2", user: "deploy"}
  db1:  {type: "ssh", host: "10.0.0.10", user: "deploy"}
}
```

---

## Groups

> **Status: Design — not yet implemented**

### What Groups Are

Groups are **explicit lists** of target names. They live separately from targets.

```cue
// groups/groups.cue
package groups

web: ["web1", "web2"]
db:  ["db1"]
all: ["web1", "web2", "db1"]
```

### Why Not Tags

Ansible's tags are OR-based and become a dumping ground for metadata. You cannot express "web AND prod" cleanly.

Groups are explicit. Composition is CUE:

```cue
// AND: intersection
prod_web: [for t in web if list.Contains(prod, t) {t}]

// OR: concatenation
web_or_db: web + db
```

### Computed Groups (Optional)

If explicit lists are too verbose:

```cue
// Computed from target properties (if you add them)
web_computed: [for name, t in targets if strings.HasPrefix(name, "web") {name}]
```

But prefer explicit lists for clarity.

---

## Variables

> **Status: Design — not yet implemented**

### The Ansible Trap

Ansible has 22 levels of variable precedence. "Where did this value come from?" is often unanswerable.

### doit Approach: Explicit Imports

Variables are layered via explicit CUE imports:

```cue
// vars/base.cue
package vars

#Base: {
  nginx_workers: 4
  log_level:     "info"
}
```

```cue
// vars/prod.cue
package vars

import "vars/base"

#Prod: base.#Base & {
  log_level:  "warn"                    // override
  ssl_cert:   "/etc/ssl/prod.crt"
  db_host:    "prod-db.internal"
}
```

```cue
// vars/prod-west.cue
package vars

import "vars/prod"

#ProdWest: prod.#Prod & {
  db_host: "prod-west-db.internal"      // regional override
}
```

### Key Properties

| Property | How |
|----------|-----|
| No magic precedence | Layering is explicit CUE imports |
| Obvious provenance | Follow the imports to see where values come from |
| Editor support | LSP can jump-to-definition through the chain |
| Type-safe | Unit's `#Params` validates what you pass |

### Vars vs Unit Params

- **Vars are freeform** — they're just CUE structs
- **Units have schemas** — `#Params` is the contract
- CUE errors at evaluation time if vars don't satisfy unit params

### Secrets (Future)

Secrets are referenced symbolically, resolved at runtime:

```cue
#Prod: #Base & {
  ssl_cert: "/etc/ssl/prod.crt"                      // literal
  db_pass:  {secret: "vault:prod/db/password"}       // runtime resolution
}
```

Resolution strategies (planned):
- SSH agent
- OS keychain
- Vault / secret manager
- Environment variables

---

## Site Definition

### Host-Centric Model

> **Note:** Unit references (e.g. `nginx.#Unit`) in examples below are aspirational. Only inline steps work today.

The site file defines "on these targets, run these steps":

```cue
// site.cue
package site

import (
  "groups"
  "units/nginx"
  "units/app"
  "vars/prod"
)

deploy: {
  web_servers: {
    targets: groups.web

    steps: [
      // Inline step
      {copy: {src: "motd.txt", dest: "/etc/motd"}},

      // Unit (expands to its steps)
      nginx.#Unit & {params: prod.#Vars},

      // More inline steps
      {template: {src: "app.conf.tmpl", dest: "/etc/app/config.yml"}},

      // Another unit
      app.#Unit & {params: prod.#Vars},
    ]
  }

  db_servers: {
    targets: groups.db

    steps: [
      postgres.#Unit & {params: prod.#Vars},
    ]
  }
}
```

### Why Host-Centric

Reading order matches execution: "On web_servers, run: copy, nginx, template, app."

Units interleave naturally with inline steps. No separate "binding" layer to reason about.

---

## Project Layout

### Principle: Complexity is Opt-In

Start with one file. Split when it hurts.

### Minimal (Single File)

```cue
// dotfiles.cue
target: {type: "local"}

steps: [
  {symlink: {src: "bashrc", dest: "~/.bashrc"}},
  {symlink: {src: "vimrc", dest: "~/.vimrc"}},
]
```

```bash
doit apply dotfiles.cue
```

### Small (One File, Multiple Targets)

```cue
// site.cue
targets: {
  laptop: {type: "local"}
  server: {type: "ssh", host: "myserver.com", user: "me"}
}

deploy: {
  dotfiles: {
    targets: ["laptop", "server"]
    steps: [
      {symlink: {src: "bashrc", dest: "~/.bashrc"}},
    ]
  }
}
```

### Medium (Split Files)

```
├── inventory/
│   └── prod.cue
├── vars/
│   └── prod.cue
└── site.cue
```

### Large (Full Layout)

```
myproject/
├── cue.mod/
│   └── module.cue
├── inventory/
│   ├── test.cue
│   ├── stage.cue
│   └── prod.cue
├── groups/
│   └── groups.cue
├── vars/
│   ├── base.cue
│   ├── test.cue
│   ├── stage.cue
│   └── prod.cue
├── units/
│   ├── nginx/
│   │   ├── unit.cue
│   │   └── templates/
│   └── app/
│       ├── unit.cue
│       └── templates/
└── site.cue
```

---

## CLI Flags

| Flag | Meaning |
|------|---------|
| (none) | Target must be inline, or defaults to local |
| `-i <file>` | Explicit inventory file |
| `--env <name>` | Convention: loads `inventory/<name>.cue` (and `vars/<name>.cue`) |
| `--targets <names>` | Filter to specific target names |
| `--only <blocks>` | Filter to specific deploy blocks |

### Examples

```bash
# Minimal: single file, local target
doit apply dotfiles.cue

# Explicit inventory
doit apply -i inventory/prod.cue site.cue

# Environment convention
doit apply --env prod site.cue

# Filter to specific targets
doit apply --env prod --targets web1,web2 site.cue

# Filter to specific deploy block
doit apply --env prod --only web_servers site.cue
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

The target IS the truth. You cannot avoid inspecting it. Caching that inspection is a liability, not an optimization.

---

## Summary

| Concept | Where | Key property |
|---------|-------|--------------|
| Units | `units/*/unit.cue` | Pure CUE, explicit `#Params` |
| Steps | Built-in | Batteries included |
| Targets | `inventory/*.cue` | Pure identity, one file per env |
| Groups | `groups/*.cue` | Explicit lists |
| Vars | `vars/*.cue` | Explicit imports, no precedence |
| Site | `site.cue` | Host-centric: "on X run Y" |

Complexity scales from one file to full layout. Explicit over implicit. Target is truth.
