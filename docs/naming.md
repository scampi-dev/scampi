# Naming Conventions and Conceptual Model

This document defines the **authoritative naming conventions** used throughout the `doit` codebase.

Its purpose is to:

* make the conceptual model explicit
* keep CUE and Go terminology aligned without conflation
* prevent naming drift and "Impl-suffix creep"
* provide a stable vocabulary for future extensions

If a naming question arises, this document is the source of truth.

---

## 1. High-level mental model

`doit` is a **declarative system convergence engine**.

Users describe *what should exist* in **CUE**.
The engine translates this into *how to make it so* in **Go**.

The core flow is:

```
CUE unit (kind + fields)
        ↓
Go UnitType
        ↓
Action (planned execution)
        ↓
Ops (execution graph)
        ↓
Target (execution environment)
```

Every name in the codebase exists to make this flow obvious.

---

## 2. CUE-side terminology (authoritative)

CUE is the **user-facing, declarative language**.
Go adapts to CUE concepts — not the other way around.

### Unit

A **unit** is a *single declarative unit of work* defined by the user.

Example:

```cue
units: [
    builtin.copy & {
        name:  "copy config"
        src:   "./src.yml"
        dest:  "/etc/app/config.yml"
        owner: "root"
        group: "root"
        mode:  "rw-r-----"
    },
]
```

### Kind

A **kind** identifies *what type of unit this is*.

* Stored as `meta.kind`
* Examples: `"copy"`, `"service"`, `"package"`

Kinds are semantic categories, not implementations.

---

## 3. Go-side core concepts

Go code mirrors the CUE model, but separates **definition**, **instance**, and **execution**.

### UnitType

A **UnitType** represents a CUE *kind* in Go.

It defines:

* how to decode configuration
* how to plan execution

```go
type UnitType interface {
    Kind() string
    NewConfig() any
    Plan(idx int, cfg any) (Action, error)
}
```

Key points:

* There is exactly one `UnitType` per CUE kind
* `UnitType` is *not* an implementation detail
* Avoid `Impl`, `Handler`, or `Spec` suffixes

> A unit has a **type**, not an "implementation".

---

### UnitInstance

A **UnitInstance** represents one concrete unit declared by the user.

```go
type UnitInstance struct {
    Name   string
    Type   UnitType
    Config any
}
```

* Created by decoding user CUE
* Couples user data with its `UnitType`
* Exists only during planning

---

### Registry

The **Registry** maps CUE kinds to Go `UnitType`s.

```go
type Registry struct {
    types map[string]spec.UnitType
}
```

Responsibilities:

* resolve kinds (`"copy" → Copy`)
* act as the extension point in the future

The registry is intentionally explicit and centralized.

---

## 4. Planning and execution

### Action

An **Action** is the planned execution of one unit instance.

```go
type Action interface {
    Name() string
    Ops() []Op
}
```

* Produced by `UnitType.Plan`
* Still declarative
* Not yet tied to a target

---

### Op

An **Op** is the smallest executable step.

```go
type Op interface {
    Name() string
    Check(ctx, target)
    Execute(ctx, target)
    DependsOn() []Op
}
```

* Ops form a DAG
* Ops are target-aware
* Ops are idempotent

This is where concurrency and dependency resolution happen.

---

### Plan

A **Plan** is an ordered collection of actions.

```go
type Plan struct {
    Actions []Action
}
```

* Produced from a config
* Executed sequentially at the action level
* Parallelism exists *within* actions via ops

---

## 5. Target

A **Target** represents the execution environment.

```go
type Target interface {
    Filesystem
    Ownership
}
```

Examples:

* `LocalPosixTarget`
* (future) `SshPosixTarget`
* (future) `WindowsTarget`

Key rule:

> Ops must treat the target as the **authority for system behavior**.

Platform-specific logic belongs in targets, not ops.

---

## 6. Package and directory naming

### Go package rules

* packages are **singular nouns**
* names describe *what the package contains*, not how it's used
* avoid suffixes like `impl`, `handler`, `spec`

### Canonical directories

```
unit/          # UnitType implementations (one per kind)
engine/        # Planning and execution engine
spec/          # Core domain interfaces and types
target/        # Execution environments
cue/           # Embedded CUE schema
```

Example:

```
unit/
  copy/
    copy.go   # type Copy implements UnitType
```

---

## 7. Explicit non-goals

The following patterns are intentionally avoided:

* `Impl` suffixes (Java-style indirection)
* Interface/implementation name pairs
* Go names leaking into CUE
* CUE schema shaped around Go constraints
* Over-generalization "for future extensions"

Extensibility is achieved by **clear boundaries**, not abstractions.

---

## 8. Summary table

| Concept           | CUE  | Go           |
| ----------------- | ---- | ------------ |
| Declarative work  | unit | UnitInstance |
| Semantic category | kind | UnitType     |
| Planned execution | —    | Action       |
| Executable step   | —    | Op           |
| Execution env     | —    | Target       |

---

## 9. Final rule of thumb

If you are unsure how to name something, ask:

> "Where does this live in the
> **unit → type → action → op → target** flow?"

If the name doesn't answer that clearly, it's wrong.
