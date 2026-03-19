# Naming Conventions

This document defines the **authoritative naming conventions** used throughout the `scampi` codebase.

Its purpose is to:
- Make the conceptual model explicit
- Keep Starlark and Go terminology aligned without conflation
- Prevent naming drift and "Impl-suffix creep"
- Provide a stable vocabulary for future extensions

If a naming question arises, this document is the source of truth.

---

## Mental Model

`scampi` is a **declarative system convergence engine**.

Users describe *what should exist* in Starlark. The engine translates this into *how to make it so* in Go.

The core flow:

```
Starlark step (kind + fields)
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
Source + Target (source-side data, target-side mutations)
```

Every name in the codebase exists to make this flow obvious.

---

## Starlark-Side Terminology

Starlark is the user-facing, declarative language. Go adapts to Starlark concepts — not the other way around.

### Step

A **step** is a single declarative unit of work defined by the user.

```python
steps = [
    copy(
        src="./src.yml",
        dest="/etc/app/config.yml",
        owner="root",
        perm="0640",
    ),
]
```

### Kind

A **kind** identifies what type of step this is.

- Returned by `StepType.Kind()`
- Examples: `"copy"`, `"dir"`, `"pkg"`, `"symlink"`, `"template"`

Kinds are semantic categories, not implementations.

---

## Go-Side Core Concepts

Go code mirrors the Starlark model, but separates **definition**, **instance**, and **execution**.

### StepType

A **StepType** represents a step kind in Go. It defines how to decode configuration and how to plan execution.

```go
type StepType interface {
    Kind() string
    NewConfig() any
    Plan(idx int, step StepInstance) (Action, error)
}
```

Key points:
- There is exactly one StepType per step kind
- `Plan` receives a `StepInstance` (not a raw config value)
- StepType is not an implementation detail
- Avoid `Impl`, `Handler`, or `Spec` suffixes

> A step has a **type**, not an "implementation".

### StepInstance

A **StepInstance** represents one concrete step declared by the user.

```go
type StepInstance struct {
    Desc   string
    Type   StepType
    Config any
    Source SourceSpan
    Fields map[string]FieldSpan
}
```

- Created by evaluating user Starlark
- Couples user data with its StepType
- Exists only during planning
- `Desc` is the human-readable description
- `Source` and `Fields` carry Starlark source locations for diagnostics

### TargetType

A **TargetType** is the target-side parallel to StepType. It defines how to decode target configuration and how to create a live target connection.

```go
type TargetType interface {
    Kind() string
    NewConfig() any
    Create(ctx context.Context, src source.Source, tgt TargetInstance) (target.Target, error)
}
```

### TargetInstance

A **TargetInstance** represents one concrete target declared by the user.

```go
type TargetInstance struct {
    Type   TargetType
    Config any
    Source SourceSpan
    Fields map[string]FieldSpan
}
```

### Registry

The **Registry** maps step kinds to Go StepTypes and TargetTypes.

```go
type Registry struct {
    stepTypes   map[string]spec.StepType
    targetTypes map[string]spec.TargetType
}
```

Responsibilities:
- Resolve step kinds (`"copy"` -> `Copy`)
- Resolve target kinds (`"local"` -> `Local`, `"ssh"` -> `SSH`)
- Act as the extension point

The registry is intentionally explicit and centralized.

---

## Configuration Model

### Config

A **Config** is the fully evaluated Starlark configuration.

```go
type Config struct {
    Path    string
    Targets map[string]TargetInstance
    Deploy  map[string]DeployBlock
}
```

### DeployBlock

A **DeployBlock** is one named group of steps targeting a set of machines.

```go
type DeployBlock struct {
    Name    string
    Targets []string
    Steps   []StepInstance
    Source  SourceSpan
}
```

### ResolvedConfig

A **ResolvedConfig** is a Config narrowed to one deploy block and one target — ready for planning.

```go
type ResolvedConfig struct {
    Path       string
    DeployName string
    TargetName string
    Target     TargetInstance
    Steps      []StepInstance
}
```

---

## Planning and Execution

### Action

An **Action** is the planned execution of one step instance.

```go
type Action interface {
    Desc() string
    Kind() string
    Ops() []Op
}
```

- Produced by `StepType.Plan`
- `Desc()` returns a human-readable description
- `Kind()` returns the step kind (e.g. `"copy"`)

Actions may optionally implement `Promiser` for automatic dependency inference
and check-mode deferral:

```go
type Promiser interface {
    Inputs()   []Resource    // resources this action consumes
    Promises() []Resource    // resources this action produces
}
```

**Every step implementation must declare its resources correctly.** This is
just as important as targets declaring their capabilities — if a step omits
an input, the engine can't order it after the action that produces that
resource; if it omits a promise, downstream steps can't defer errors during
check mode.

Resource types:
- `PathResource(path)` — filesystem path (supports parent-directory matching)
- `UserResource(name)` — system user account
- `GroupResource(name)` — system group

Rules:
- If a step creates a path, promise `PathResource(path)`
- If a step creates a user/group, promise the corresponding resource when
  `state=present` (not when `state=absent`)
- If a step sets `owner`/`group` fields, declare `UserResource` and
  `GroupResource` inputs
- If a step depends on a path existing (e.g. symlink target), declare a
  `PathResource` input
- If a step references supplementary groups, declare `GroupResource` inputs
  for each one
- Steps without any resources act as **barriers** — they force sequential
  execution relative to all preceding actions

### Op

An **Op** is the smallest executable unit.

```go
type Op interface {
    Action() Action
    Check(ctx context.Context, src source.Source, tgt target.Target) (CheckResult, []DriftDetail, error)
    Execute(ctx context.Context, src source.Source, tgt target.Target) (Result, error)
    DependsOn() []Op
    RequiredCapabilities() capability.Capability
}
```

- Ops form a DAG (parallelism via the scheduler)
- Ops receive both a Source (read state) and a Target (apply mutations)
- Ops are idempotent
- `RequiredCapabilities()` declares what the target must support
- `Action()` links back to the parent action

Ops may optionally implement `OpDescriber` for self-describing plan output:

```go
type OpDescriber interface {
    OpDescription() OpDescription
}

type OpDescription interface {
    PlanTemplate() PlanTemplate
}

type PlanTemplate struct {
    ID   string
    Text string
    Data any
}
```

### CheckResult

`Check` returns a `CheckResult` indicating current state:

```go
type CheckResult uint8

const (
    CheckUnknown     CheckResult = iota // could not determine
    CheckSatisfied                      // already correct
    CheckUnsatisfied                    // needs execution
)
```

### Plan

A **Plan** contains a single Unit ready for execution.

```go
type Plan struct {
    Unit Unit
}
```

### Unit

A **Unit** groups a target with its ordered actions.

```go
type Unit struct {
    ID      UnitID
    Desc    string
    Target  target.Target
    Actions []Action
}
```

- `ID` uniquely identifies the unit (deploy block + target name)
- Actions are executed sequentially
- Parallelism exists within actions via ops

---

## Source and Target

### Source

A **Source** is where configuration data originates.

```go
type Source interface {
    ReadFile(ctx context.Context, path string) ([]byte, error)
    WriteFile(ctx context.Context, path string, data []byte) error
    EnsureDir(ctx context.Context, path string) error
    Stat(ctx context.Context, path string) (FileMeta, error)
    LookupEnv(key string) (string, bool)
    LookupSecret(key string) (string, bool, error)
}
```

- Supports both read and write (write is for caching, not mutation)
- `LookupEnv` provides environment variable access
- `LookupSecret` resolves secrets from the configured backend
- Examples: local filesystem, git repository, S3 bucket
- Used to fetch files that will be distributed to targets

### Target

A **Target** is where changes are applied. Targets use capability-based composition.

```go
type Target interface {
    Capabilities() capability.Capability
}
```

Targets advertise capabilities. Ops declare what they need via `RequiredCapabilities()`. The engine validates compatibility at plan time.

Available capability interfaces:

```go
type Filesystem interface { ... }  // ReadFile, WriteFile, Stat, Remove
type FileMode interface { ... }    // Chmod, mode in Stat
type Symlink interface { ... }     // Symlink, Readlink, Lstat
type Ownership interface { ... }   // HasUser, HasGroup, GetOwner, Chown
type Pkg interface { ... }         // IsInstalled, InstallPkgs, RemovePkgs
type PkgUpdate interface { ... }   // UpdateCache, IsUpgradable
type Service interface { ... }     // IsActive, IsEnabled, Start, Stop, Enable, Disable
type Command interface { ... }     // RunCommand
```

The capability bitmask:

```go
const (
    Filesystem Capability = 1 << iota
    FileMode
    Symlink
    Ownership
    Pkg
    PkgUpdate
    Service
    Command
)
```

- The managed system(s)
- Examples: local POSIX filesystem, SSH remote

> Ops must treat the target as the **authority for system behavior**.

Platform-specific logic belongs in targets, not ops.

### Source vs Target

| Aspect       | Source                    | Target                       |
|--------------|---------------------------|------------------------------|
| Role         | Data origin               | Change destination           |
| Read         | Yes                       | Yes                          |
| Write        | For caching only          | For mutations                |
| Multiplicity | Typically one             | Often many                   |
| Interface    | Single concrete interface | Capability-based composition |

Both are abstract interfaces. The local POSIX implementations are development defaults, not the only options.

---

## Package and Directory Naming

### Go Package Rules

- Packages are singular nouns
- Names describe what the package contains, not how it's used
- Avoid suffixes like `impl`, `handler`, `spec`

### Canonical Directories

```
cmd/           # CLI entry point
engine/        # Planning and execution engine
spec/          # Core domain interfaces and types
star/          # Starlark evaluator and builtins
step/          # StepType implementations (one per kind)
source/        # Source-side access: configs, env, and local cache
target/        # Execution environments (write-side mutations)
capability/    # Capability system for target/op matching
diagnostic/    # Event emission (observational only, no control flow)
render/        # CLI output formatting (purely presentational)
model/         # Execution reports and op outcomes
signal/        # Severity, verbosity, and color mode
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
- Go names leaking into Starlark
- Starlark builtins shaped around Go constraints
- Over-generalization "for future extensions"

Extensibility is achieved by **clear boundaries**, not abstractions.

---

## Summary

| Concept              | Starlark                    | Go                                    |
|----------------------|-----------------------------|---------------------------------------|
| Declarative work     | step builtin (copy, dir, …) | StepInstance                          |
| Semantic category    | kind                        | StepType                              |
| Target definition    | target.ssh / target.local   | TargetInstance                        |
| Target category      | target type                 | TargetType                            |
| Planned execution    | —                           | Action                                |
| Executable unit      | —                           | Op                                    |
| Execution graph      | —                           | Plan / Unit                           |
| Data origin          | —                           | Source                                |
| Change destination   | —                           | Target (capability-based)             |
| Configuration        | deploy()                    | Config / DeployBlock / ResolvedConfig |
| Check outcome        | —                           | CheckResult                           |
| Op self-description  | —                           | OpDescriber / PlanTemplate            |
| Dependency inference | —                           | Promiser                              |

---

## Rule of Thumb

If you are unsure how to name something, ask:

> "Where does this live in the **step -> type -> action -> op -> target** flow?"

If the name doesn't answer that clearly, it's wrong.
