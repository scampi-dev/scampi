# Naming Conventions

This document defines the **authoritative naming conventions** used throughout the `doit` codebase.

Its purpose is to:
- Make the conceptual model explicit
- Keep CUE and Go terminology aligned without conflation
- Prevent naming drift and "Impl-suffix creep"
- Provide a stable vocabulary for future extensions

If a naming question arises, this document is the source of truth.

---

## Mental Model

`doit` is a **declarative system convergence engine**.

Users describe *what should exist* in CUE. The engine translates this into *how to make it so* in Go.

The core flow:

```
CUE step (kind + fields)
        |
        v
Go StepType
        |
        v
Action (planned execution)
        |
        v
Ops (execution graph)
        |
        v
Target (execution environment)
```

Every name in the codebase exists to make this flow obvious.

---

## CUE-Side Terminology

CUE is the user-facing, declarative language. Go adapts to CUE concepts — not the other way around.

### Step

A **step** is a single declarative unit of work defined by the user.

```cue
steps: [
  builtin.copy & {
    src:   "./src.yml"
    dest:  "/etc/app/config.yml"
    owner: "root"
    mode:  "0640"
  },
]
```

### Kind

A **kind** identifies what type of step this is.

- Stored as `meta.kind`
- Examples: `"copy"`, `"service"`, `"package"`

Kinds are semantic categories, not implementations.

---

## Go-Side Core Concepts

Go code mirrors the CUE model, but separates **definition**, **instance**, and **execution**.

### StepType

A **StepType** represents a CUE kind in Go. It defines how to decode configuration and how to plan execution.

```go
type StepType interface {
    Kind() string
    NewConfig() any
    Plan(idx int, cfg any) (Action, error)
}
```

Key points:
- There is exactly one StepType per CUE kind
- StepType is not an implementation detail
- Avoid `Impl`, `Handler`, or `Spec` suffixes

> A step has a **type**, not an "implementation".

### StepInstance

A **StepInstance** represents one concrete step declared by the user.

```go
type StepInstance struct {
    Name   string
    Type   StepType
    Config any
}
```

- Created by decoding user CUE
- Couples user data with its StepType
- Exists only during planning

### Registry

The **Registry** maps CUE kinds to Go StepTypes.

```go
type Registry struct {
    types map[string]spec.StepType
}
```

Responsibilities:
- Resolve kinds (`"copy"` -> `Copy`)
- Act as the extension point

The registry is intentionally explicit and centralized.

---

## Planning and Execution

### Action

An **Action** is the planned execution of one step instance.

```go
type Action interface {
    Name() string
    Ops() []Op
}
```

- Produced by `StepType.Plan`
- Still declarative
- Not yet tied to a target

### Op

An **Op** is the smallest executable unit.

```go
type Op interface {
    Name() string
    Check(ctx, target)
    Execute(ctx, target)
    DependsOn() []Op
}
```

- Ops form a DAG
- Ops are target-aware
- Ops are idempotent

This is where concurrency and dependency resolution happen.

### Plan

A **Plan** is an ordered collection of actions.

```go
type Plan struct {
    Actions []Action
}
```

- Produced from a config
- Executed sequentially at the action level
- Parallelism exists within actions via ops

---

## Source and Target

### Source

A **Source** is where configuration data originates.

```go
type Source interface {
    ReadFile(ctx, path) ([]byte, error)
    WriteFile(ctx, path, data) error   // for caching
    EnsureDir(ctx, path) error         // for caching
    Stat(ctx, path) (FileMeta, error)
}
```

- Supports both read and write (write is for caching, not mutation)
- Examples: local filesystem, git repository, S3 bucket
- Used to fetch files that will be distributed to targets

### Target

A **Target** is where changes are applied.

```go
type Target interface {
    Filesystem
    Ownership
}
```

- The managed system(s)
- Examples: local filesystem, SSH remote, REST API

> Ops must treat the target as the **authority for system behavior**.

Platform-specific logic belongs in targets, not ops.

### Source vs Target

| Aspect | Source | Target |
|--------|--------|--------|
| Role | Data origin | Change destination |
| Read | Yes | Yes |
| Write | For caching only | For mutations |
| Multiplicity | Typically one | Often many |

Both are abstract interfaces. The local POSIX implementations are development defaults, not the only options.

---

## Package and Directory Naming

### Go Package Rules

- Packages are singular nouns
- Names describe what the package contains, not how it's used
- Avoid suffixes like `impl`, `handler`, `spec`

### Canonical Directories

```
step/          # StepType implementations (one per kind)
engine/        # Planning and execution engine
spec/          # Core domain interfaces and types
target/        # Execution environments
cue/           # Embedded CUE schema
```

Example:

```
step/
  copy/
    copy.go   # type Copy implements StepType
```

---

## Explicit Non-Goals

The following patterns are intentionally avoided:

- `Impl` suffixes (Java-style indirection)
- Interface/implementation name pairs
- Go names leaking into CUE
- CUE schema shaped around Go constraints
- Over-generalization "for future extensions"

Extensibility is achieved by **clear boundaries**, not abstractions.

---

## Summary

| Concept | CUE | Go |
|---------|-----|-----|
| Declarative work | step | StepInstance |
| Semantic category | kind | StepType |
| Planned execution | — | Action |
| Executable unit | — | Op |
| Data origin | — | Source |
| Change destination | — | Target |

---

## Rule of Thumb

If you are unsure how to name something, ask:

> "Where does this live in the **step -> type -> action -> op -> target** flow?"

If the name doesn't answer that clearly, it's wrong.
